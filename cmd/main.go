/*
Copyright 2025 Thibaut Lapierre.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package main is the entry point for the VDI controller
package main

import (
	"crypto/tls"
	"errors"
	"flag"
	"os"
	"time"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/filters"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	vdiv1alpha1 "github.com/epheo/deskube/api/v1alpha1"
	"github.com/epheo/deskube/internal/controller"
	// +kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(vdiv1alpha1.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	var secureMetrics bool
	var enableHTTP2 bool
	var tlsOpts []func(*tls.Config)
	flag.StringVar(&metricsAddr, "metrics-bind-address", "0", "The address the metrics endpoint binds to. "+
		"Use :8443 for HTTPS or :8080 for HTTP, or leave as 0 to disable the metrics service.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	// OpenShift's values for single node clusters: a loaded node renews late.
	// controller-runtime's 15s, 10s and 2s made the manager exit on 2026-10-31.
	var leaseDuration, renewDeadline, retryPeriod time.Duration
	flag.DurationVar(&leaseDuration, "leader-elect-lease-duration", 137*time.Second,
		"How long a candidate waits before taking a lease the leader stopped renewing.")
	flag.DurationVar(&renewDeadline, "leader-elect-renew-deadline", 107*time.Second,
		"How long the leader keeps retrying a renewal before it gives up and exits.")
	flag.DurationVar(&retryPeriod, "leader-elect-retry-period", 26*time.Second,
		"How long to wait between attempts to take or renew the lease.")
	flag.BoolVar(&secureMetrics, "metrics-secure", true,
		"If set, the metrics endpoint is served securely via HTTPS. Use --metrics-secure=false to use HTTP instead.")
	flag.BoolVar(&enableHTTP2, "enable-http2", false,
		"If set, HTTP/2 will be enabled for the metrics server")
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	var probeExternal bool
	flag.BoolVar(&probeExternal, "probe-external", false,
		"Reach Sunshine through the client address instead of the pod IP, for a manager outside the cluster. "+
			"Dev only: every desktop's Service then publishes its web UI port, webUI or not.")
	var sandboxSeccompProfile string
	flag.StringVar(&sandboxSeccompProfile, "sandbox-seccomp-profile", "",
		"Seccomp profile already on the nodes, relative to the kubelet's seccomp directory, for desktops with "+
			"nestedSandboxes. It must allow nested namespaces. Empty lets the Security Profiles Operator install "+
			"the built in one, and leaves those desktops without a filter where that operator is absent.")
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))
	if probeExternal {
		setupLog.Info("probe-external: reaching desktops through their client address; " +
			"every desktop's Service publishes its web UI port")
	}
	// kustomize picks the namespace, so the pod is told it.
	managerNamespace := os.Getenv("POD_NAMESPACE")
	if managerNamespace == "" && !probeExternal {
		setupLog.Error(errors.New("POD_NAMESPACE is not set"),
			"desktops admit the manager to their admin API by its namespace; "+
				"set it, or pass --probe-external outside the cluster")
		os.Exit(1)
	}

	// if the enable-http2 flag is false (the default), http/2 should be disabled
	// due to its vulnerabilities. More specifically, disabling http/2 will
	// prevent from being vulnerable to the HTTP/2 Stream Cancellation and
	// Rapid Reset CVEs. For more information see:
	// - https://github.com/advisories/GHSA-qppj-fm5r-hxr3
	// - https://github.com/advisories/GHSA-4374-p667-p6c8
	disableHTTP2 := func(c *tls.Config) {
		setupLog.Info("disabling http/2")
		c.NextProtos = []string{"http/1.1"}
	}

	if !enableHTTP2 {
		tlsOpts = append(tlsOpts, disableHTTP2)
	}

	// Metrics endpoint is enabled in 'config/default/kustomization.yaml'. The Metrics options configure the server.
	// More info:
	// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.0/pkg/metrics/server
	// - https://book.kubebuilder.io/reference/metrics.html
	metricsServerOptions := metricsserver.Options{
		BindAddress:   metricsAddr,
		SecureServing: secureMetrics,
		TLSOpts:       tlsOpts,
	}

	if secureMetrics {
		// FilterProvider is used to protect the metrics endpoint with authn/authz.
		// These configurations ensure that only authorized users and service accounts
		// can access the metrics endpoint. The RBAC are configured in 'config/rbac/kustomization.yaml'. More info:
		// https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.0/pkg/metrics/filters#WithAuthenticationAndAuthorization
		metricsServerOptions.FilterProvider = filters.WithAuthenticationAndAuthorization
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		Cache:                  cache.Options{ByObject: controller.CacheByObject()},
		Metrics:                metricsServerOptions,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "1e73960d.desku.be",
		LeaseDuration:          &leaseDuration,
		RenewDeadline:          &renewDeadline,
		RetryPeriod:            &retryPeriod,
		// A rollout would otherwise wait out the lease.
		// Safe only because the process exits as soon as the manager stops.
		LeaderElectionReleaseOnCancel: true,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	if err = (&controller.VdiInstanceReconciler{
		Client:                mgr.GetClient(),
		Scheme:                mgr.GetScheme(),
		ProbeExternal:         probeExternal,
		ManagerNamespace:      managerNamespace,
		APIReader:             mgr.GetAPIReader(),
		SandboxSeccompProfile: sandboxSeccompProfile,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "VdiInstance")
		os.Exit(1)
	}

	// +kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
