# GPU desktops on NVIDIA

Design for DeskuBe desktops on a Kubernetes node with an NVIDIA GPU.
Written 2026-11-08, before anyone had the GPU in hand.
Nothing here has run on an NVIDIA GPU yet.
The plan at the end says what to run, in order.

Marks:

- (V): verified, read in source code or vendor docs, cited.
- (I high), (I med), (I low): inference, with my confidence.

Source code was read at the versions the target node and the images pin:
nvidia-container-toolkit v1.20.1, k8s-device-plugin v0.20.1,
Sunshine v2026.914.233613, open-gpu-kernel-modules 615.71.09,
wlroots, sway and Mesa at master on 2026-11-08.
Links are in Sources, numbered [S1] and on.

## Answers in short

1. Rendering.
   sway on the wlroots headless backend should run on the NVIDIA driver through GBM, unprivileged. (I med)
   It needs nvidia-drm modeset=1, the default at 615.71.09. (V)
   The CDI spec injects every node it needs. (V)
   No `--unsupported-gpu`: sway's check never fires on the headless backend. (V)
   `WLR_RENDERER=gles2`, not vulkan. (I med)
2. Capture and encode.
   Sunshine 2026.914 captures wlr into a GBM dmabuf, imports it into EGL, hands it to CUDA, and NVENC encodes it. No system memory copy. (V)
   That path opens the primary node `/dev/dri/cardN`. (V)
   On headless NVIDIA its GBM capture buffer has failed for others; Sunshine fixed both after our pin, first in pre-release v2026.1107. (V)
   Fallback: `capture = portal` with `encoder = vulkan`. (V that the code path exists; I low that it works on NVIDIA)
3. Driver userspace.
   The device plugin writes one CDI spec; CRI-O applies it: nodes, read-only library mounts, config files, hooks run as root, extra groups. (V)
   `NVIDIA_DRIVER_CAPABILITIES` does nothing under CDI, neither the plugin's nor the pod's. (V)
   NVIDIA's packages for the driver already carry every graphics and video library the spec needs. (V)
   No node change for capabilities; SELinux and time slicing do need one.
   Mesa stays in the image; pin EGL and Vulkan to NVIDIA with two variables. (I high)
4. Sharing.
   Time slicing only hands the same GPU to N pods: no memory isolation, no fair share, one fault domain. (V)
   A pod asks `limits: {nvidia.com/gpu.shared: 1}`. (V)
   GeForce RTX 3080: one NVENC, 12 concurrent sessions per NVIDIA's matrix. (V)
5. Security.
   No capability, no privileged pod, RuntimeDefault seccomp. (I med)
   Cost: the NVIDIA driver's ioctl surface, the card node, and one SELinux rule on the node for `xserver_misc_device_t`. (V)
6. Steam.
   gamescope stays nested in sway; the card node it needs is in the spec. (V)
   NVIDIA's EL10 repository has no 32-bit userspace, so 32-bit games get no GPU driver. (V)

## The node it was designed against

A single node, as described here; the design holds for any node like it.

- MicroShift 4.22 (OKD) on CentOS Stream CoreOS 10: CRI-O, SELinux enforcing, SCCs on top of Pod Security.
- GeForce RTX 3080 (Ampere, 10 GB).
- NVIDIA open kernel modules 615.71.09, built for the node's kernel.
- NVIDIA userspace from NVIDIA's EL10 CUDA repository: nvidia-driver-libs, nvidia-driver-cuda; from EPEL: egl-gbm, egl-wayland2, egl-x11.
- nvidia-container-toolkit-base 1.20.1, with `nvidia-cdi-refresh` enabled.
- nvidia-device-plugin 0.20.1: `DEVICE_LIST_STRATEGY=cdi-cri`, privileged, `NVIDIA_DRIVER_CAPABILITIES=compute,utility`, no config file, so no sharing.
- No nvidia container runtime configured; CRI-O injects CDI itself.

What those packages hold (V, NVIDIA's EL10 repository read 2026-11-08):

- nvidia-driver-libs 615.71.09: libEGL_nvidia, libGLX_nvidia, libnvidia-allocator, `/usr/lib64/gbm/nvidia-drm_gbm.so`, `10_nvidia.json`, `nvidia_icd.x86_64.json`.
- nvidia-driver-cuda-libs 615.71.09: libcuda, libnvidia-encode, libnvcuvid.
- No i686 package in the repository at all.
- nvidia-kmod-common 615.71.09, `/usr/lib/modprobe.d/nvidia.conf`: `softdep nvidia post: nvidia-uvm nvidia-drm`, no modeset option.
- nvidia-container-toolkit-base 1.20.1, `nvidia-cdi-refresh.service`: creates the control nodes, then runs `nvidia-ctk cdi generate` into `/var/run/cdi/nvidia.yaml`.
- nvidia-driver-selinux 0.1-3, pulled in by nvidia-kmod-common: only lets `systemd_sleep_t` use tmp files (decompiled with `/usr/libexec/selinux/hll/pp`). Nothing for containers.

### Images (V, desku.be-images repository)

- `desk-session:71` defaults the render node to `/dev/dri/renderD128`.
- `desk-session:87-96`: with a render node, wlroots on it, `encoder = vaapi`, `capture = wlr`; without, pixman, portal, software.
- `desk-session:155-171` writes `adapter_name = $RENDER` into sunshine.conf.
- `base/Containerfile` installs Mesa (GL, Vulkan, VAAPI freeworld), sway, Sunshine from `base/pins`, and strips file capabilities.
- `steam/` needs the primary node to exist for gamescope, and an unmasked /proc.

### Operator (V)

- `fillPodSpec` copies `spec.resources` into the container as is (`internal/controller/vdiinstance_controller.go:508`).
- `spec.stream.encoder` becomes `DESKUBE_ENCODER` (:525-527); the enum is `software;vaapi` (`api/v1alpha1/vdiinstance_types.go:72`).
- The pod drops all capabilities and runs RuntimeDefault seccomp, or a sandbox profile with `nestedSandboxes` (:542-550).

## 1. Rendering

### 1.1 sway headless on NVIDIA, unprivileged, through GBM

How wlroots gets a GPU on the headless backend (V):

- It opens the render node in `WLR_RENDER_DRM_DEVICE` and allocates its buffers there through GBM.
- EGL: it first looks for the EGL device matching that node, then falls back to the GBM platform (wlroots `render/egl.c:584-641`).
- If a matching EGL device is found but fails to initialise, it gives up without trying GBM (`egl.c:598-604`). See 3.5.

How GBM reaches NVIDIA (V):

- Mesa's libgbm loads `<DRM driver name>_gbm.so`, here `nvidia-drm_gbm.so`, from `GBM_BACKENDS_PATH` or its default directory (Mesa `src/gbm/main/backend.c`, `_gbm_create_device`).
- The CDI hook links `/usr/lib64/gbm/nvidia-drm_gbm.so` to `../libnvidia-allocator.so.1` in the container (toolkit `internal/discover/graphics.go:235-270`).
- NVIDIA's GBM backend needs DRM KMS enabled and Mesa libgbm 21.2 or later [S1].
- KMS means nvidia-drm modeset=1. At 615.71.09 modeset and fbdev default to true (`kernel-open/nvidia-drm/nvidia-drm-os-interface.c:45-46` [S2]).
- NVIDIA's packages override neither, and load nvidia-drm with nvidia through the softdep.

Who runs this today:

- sunshine-headless-sway: headless sway, Sunshine wlr capture, NVENC, on NVIDIA, as a user service, with `WLR_RENDERER=gles2` [S3]. Not in a container. (V that the project says so)
- labwc headless with Sunshine on RTX 4060 Ti and 5060 Ti, TrueNAS [S4].
- No report found of either inside an unprivileged container.

Verdict: it works. (I med) Proof: E3.

### 1.2 Driver, modeset, device nodes

- Driver: GBM support since 495 [S1]; explicit sync since 555 [S5]. 615.71.09 has both.
- Open or proprietary kernel module: same userspace; the RTX 3080 (Ampere) runs the open one. (I high)
- modeset=1: default, see 1.1. Check `/sys/module/nvidia_drm/parameters/modeset` in E0.
- Nodes the plugin's CDI spec injects for one GPU (V, toolkit `pkg/nvcdi/common-nvml.go:52-62`, `internal/platform-support/dgpu/nvml.go:48-53`):
  - `/dev/nvidiactl`, `/dev/nvidia-uvm`, `/dev/nvidia-uvm-tools`, `/dev/nvidia-modeset`;
  - `/dev/nvidia0`;
  - `/dev/dri/cardN` and `/dev/dri/renderDN` of that GPU, when nvidia-drm has created them.
- What uses what (I med):
  - EGL, GBM, Vulkan: nvidiactl, nvidia0, nvidia-modeset, renderD.
  - CUDA and NVENC: nvidiactl, nvidia0, nvidia-uvm.
  - Sunshine 2026.914's CUDA path: cardN as well (V, see 2.1).
  - gamescope: cardN must exist; nothing opens it (V, steam README).
- The CDI device is all or nothing; a pod cannot drop nodes from it. (V)

### 1.3 `--unsupported-gpu`, `WLR_RENDERER=vulkan`

- Since 1.10, sway asks each backend for its DRM fd and flags `nvidia-drm` (sway 1.10 `sway/server.c:142-160`).
- It then shows a swaynag bar; it does not exit (sway master `sway/main.c:388-393`).
- The headless backend has no `get_drm_fd` (wlroots `backend/headless/backend.c:48-51`), so the check never fires. (V)
- No flag needed. `SWAY_UNSUPPORTED_GPU=true` costs nothing if an older sway shows up. (I high)
- Renderer: `WLR_RENDERER=gles2`. sunshine-headless-sway reports NVIDIA modifier trouble with the Vulkan renderer on headless [S3]. (I low that it still holds)

### 1.4 gamescope's headless backend instead

- Sunshine cannot capture gamescope (images README, "The games").
- On NVIDIA 580.82.09, `gamescope --backend headless` started but drew only the pointer [S6].
- Not pursued. gamescope stays a window of sway. (I med)

## 2. Capture and encode

### 2.1 wlr capture with NVENC in Sunshine v2026.914.233613

- The Fedora RPM is built with CUDA on x86_64, CUDA 13.1.1 for Fedora 44 and later (V, `packaging/linux/copr/Sunshine.spec:121-129, 361-366`).
- Encoder order: nvenc, vulkan, vaapi, software (V, `src/video.cpp:1450-1468`).
- With nvenc, wlr capture takes the VRAM path (V, `src/platform/linux/wlgrab.cpp:22-33, 499-512`).
- Frame path (V):
  1. Sunshine allocates a GBM buffer on `adapter_name`'s render node, with the compositor's modifiers (`wayland.cpp:259-280, 414-436`).
  2. The compositor copies its output into it (wlr-screencopy).
  3. Sunshine imports the dmabuf into EGL on a GBM device opened from the CUDA device's card node, converts to NV12 in GL, registers the textures with CUDA (`cuda.cpp:354-393, 418-440, 517-522`).
  4. FFmpeg's h264_nvenc or hevc_nvenc encodes.
- Cost: one blit in the compositor, one colour conversion, both on the GPU; no trip through system memory. (V for the path)
- Expect 2 to 4 ms per frame, as VAAPI on the Radeon. (I med)
- The card node: `open_drm_fd_for_cuda_device` opens `/dev/dri/cardN`, not renderD (`cuda.cpp:369-385`). It drops DRM master if the open granted it (`misc.cpp:193-235`, PR #5286 [S7]). (V)
- A build without CUDA would copy GPU to RAM to GPU for NVENC (`wlgrab.cpp:514-517`). (V)
- If the CUDA path fails at start, Sunshine moves to the next encoder. wlr does not take vulkan (`wlgrab.cpp:500`), vaapi has no NVIDIA encoder, so it ends on software. (I med)

Fixed upstream after our pin (V):

- PR #4946 [S4], merged 2026-11-01:
  - prefers the render node for CUDA, card node as fallback;
  - retries GBM with relaxed usage flags;
  - falls back to `wl_shm` capture when dmabuf fails.
- Its author saw Sunshine's GBM capture buffer fail on headless NVIDIA.
- It is in pre-release v2026.1107.185223 and later, not yet in a stable release. (V release list; I high that the merge is in it)
- The shm fallback costs a GPU to RAM to GPU copy: about 8 MB a frame at 1080p, 0.5 GB/s at 60 fps. (I med)

### 2.2 When wlr with NVENC fails

- KMS capture: needs `CAP_SYS_ADMIN` (V, Sunshine `docs/configuration.md:2212-2214`).
  A headless compositor never puts a frame on a plane, so there is nothing to grab either. (I high) Refused.
- NvFBC: X11 only, not Wayland nor Xwayland (V, `docs/configuration.md:2200-2203`). Out.
- Portal and PipeWire, through xdg-desktop-portal-wlr (V, `pipewire.cpp`):
  - dmabufs go to vaapi, vulkan, or nvenc when the display GPU is NVIDIA (:338-345);
  - with nvenc it is the same CUDA path as wlr (:1094-1101);
  - Sunshine calls any Intel card at `/sys/class/drm/card0` or `card1` a hybrid laptop and then copies through memory (:1364-1386); sysfs shows the host's cards, so check the node in E0;
  - with `encoder = vulkan`: dmabuf into Vulkan compute, then Vulkan Video encode, render node only, no CUDA, no card node (`vulkan_encode.cpp`, `pipewire.cpp:1088-1090`).
- Sunshine documents the vulkan encoder for NVIDIA (`docs/configuration.md:2276-2278`). Its PR was tested on an AMD iGPU [S8]. (I low that it works on NVIDIA)
- wlr with vulkan was removed in April 2026 (PR #5030 [S9]). (V)
- The portal path hung Sunshine twice on the CPU path (images README). (V)

### 2.3 Settings

First choice:

    capture = wlr
    encoder = nvenc
    adapter_name = <the NVIDIA render node>

- `nvenc_preset = 1`, the default and fastest. (V docs)
- `nvenc_twopass`: default `quarter_res`; try `disabled` when many streams share one NVENC. (I low)
- Codecs: the RTX 3080 encodes H.264 and HEVC, not AV1 [S10]. Moonlight then picks HEVC.
- Second choice: Sunshine v2026.1107 or later, same settings.
- Third: `capture = portal`, `encoder = vulkan`.

## 3. Driver userspace in the pod

### 3.1 How cdi-cri injects

- At start the plugin writes `/var/run/cdi/k8s.device-plugin.nvidia.com-gpu.json`, one device per GPU (V, plugin `internal/cdi/cdi.go:127-158, 190-231`, `cmd/nvidia-device-plugin/plugin-manager.go:60`).
- It builds it with the toolkit's nvcdi library, vendored at v1.20.1 (V, plugin `go.mod:9`).
- `Allocate` answers CDI device names in the CRI field and nothing else: no env, no mounts (V, `internal/plugin/server.go:322-352, 370-403`).
- kubelet passes them to CRI-O; `DevicePluginCDIDevices` is GA since 1.31 [S11].
- CRI-O reads `/etc/cdi` and `/var/run/cdi` by default [S12] and applies the edits.

The edits (V, toolkit v1.20.1):

- Device nodes: see 1.2.
- Read-only bind mounts, at their host paths:
  - every library ending in the driver version, found through the ld cache (`pkg/nvcdi/driver-nvml.go:293`): EGL, GLX, GLES, Vulkan core, allocator (GBM), encode, cuvid, cuda, ml;
  - egl-gbm, egl-wayland, egl-wayland2, egl-xcb, egl-xlib (`internal/discover/graphics.go:162-184`).
- Config files at standard container paths (`graphics.go:82-139`):
  - `/usr/share/glvnd/egl_vendor.d/10_nvidia.json`;
  - `/usr/share/egl/egl_external_platform.d/15_nvidia_gbm.json` and the wayland ones;
  - `/etc/vulkan/icd.d/nvidia_icd.x86_64.json`.
- nvidia-smi and other binaries, GSP firmware, the persistenced socket (`driver-nvml.go:38-76, 273-288`).
- createContainer hooks, run as root on the node (`driver-nvml.go:99-131`, `pkg/nvcdi/full-gpu-nvml.go:161-195`):
  - update-ldcache, create-symlinks (the GBM link above);
  - disable-device-node-modification: hides `ModifyDeviceFiles` so userspace never mknods;
  - update-application-profile: limits EGL to the injected GPU;
  - apply-cuda-memory-limits: see 4.1.
- Additional groups: the group of any injected node that is not world read-write (`internal/edits/device.go:119-136`).
- No runtime class. CRI-O does all of this; the node configures no nvidia runtime. (V)

### 3.2 Which capabilities a desktop needs

- In the legacy runtime's terms: graphics (EGL, GL, Vulkan, GBM), video (NVENC, NVDEC), compute (CUDA, for Sunshine's interop), utility (NVML, nvidia-smi).
- Not display: that is the X.Org driver. (I high, names from toolkit `internal/config/image/capabilities.go:47`)
- Under CDI the question does not arise:
  - `nvidia-ctk cdi generate` has no capabilities option (`cmd/nvidia-ctk/cdi/generate/generate.go:100-258`);
  - nvcdi adds the graphics mounts unconditionally (`pkg/nvcdi/common-nvml.go:27-50`);
  - the only reader of `NVIDIA_DRIVER_CAPABILITIES` left is the nvidia runtime's graphics modifier (`internal/modifier/graphics.go:71`). (V)

### 3.3 Does the plugin's setting limit what pods get

- No. The plugin's `NVIDIA_DRIVER_CAPABILITIES` would shape its own container under an nvidia runtime; the node has none, so it does nothing. The plugin's code never reads it. (V)
- The pod does not choose either: every GPU pod gets the whole spec. (V)
- Libraries are read-only mounts and grant nothing; the privilege is in the device nodes. (I high)

### 3.4 What must change on the node for graphics and video

- For capabilities: nothing. (V)
- What the spec needs from the host is installed: nvidia-driver-libs, egl-gbm, egl-wayland2, egl-x11, nvidia-driver-cuda-libs. (V)
- Two things do need a change: SELinux (5.3) and time slicing (4.1).
- The plugin generates its spec once, at start. If nvidia-drm came up later the spec lacks the DRM nodes. (I med) E0 checks.
- Later, optional: `CDI_FEATURE_FLAGS=no-additional-gids-for-device-nodes` (5.1).

### 3.5 Mesa must not shadow the injected driver

- GBM: keep Mesa's libgbm. It is the loader NVIDIA's backend plugs into, and its own backend is `dri_gbm.so`. Nothing to change. (V)
- EGL: glvnd loads vendor files in `strcmp()` order, so NVIDIA's `10_` comes before Mesa's `50_` [S13]. (V)
  wlroots gives up if the first matching device fails (1.1).
  Pin it in the NVIDIA path: `__EGL_VENDOR_LIBRARY_FILENAMES=/usr/share/glvnd/egl_vendor.d/10_nvidia.json`. Then only that file loads [S13].
- Vulkan: the loader merges `/etc/vulkan/icd.d` (NVIDIA, from CDI) and `/usr/share/vulkan/icd.d` (Mesa: radv, anv, nvk, lavapipe).
  Pin it: `VK_DRIVER_FILES=/etc/vulkan/icd.d/nvidia_icd.x86_64.json`. `VK_ICD_FILENAMES` is the deprecated name [S14].
- GLX, only for X11 apps under Xwayland in the Steam image: `__GLX_VENDOR_LIBRARY_NAME=nvidia` if GL games land on Mesa. (I low) E7.
- Never put NVIDIA userspace in the image: it must match the node's kernel module to the patch level, and CDI mounts over the same paths. (I high)

## 4. Sharing one GPU

### 4.1 Time slicing

What it is (V, plugin `README.md:358-483`):

- `sharing.timeSlicing.resources: [{name: nvidia.com/gpu, replicas: N}]` advertises N per GPU.
- "the plugin simply creates 10 references to each GPU and indiscriminately hands them out".
- `renameByDefault: true` advertises `nvidia.com/gpu.shared` instead.
- `failRequestsGreaterThanOne: true` refuses requests above 1, since 2 buys nothing.
- One method for every GPU of the node.

What it does not give (V, same README):

- "nothing special is done to isolate workloads".
- Each workload "has access to the GPU memory".
- Same "fault-domain": one crash takes all down.
- No fair share: CUDA gives "an equal share of time to all of the GPU processes", per process, not per pod.
- MPS partitions memory and compute, for CUDA clients only (:373-376). Graphics and NVENC do not go through MPS. (I high) Not for desktops.

A memory limit may exist (V for the mechanism):

- Toolkit 1.20.1 adds the apply-cuda-memory-limits hook (`CHANGELOG.md`, `cmd/nvidia-cdi-hook/apply-cuda-memory-limits/`).
- It reads `NVIDIA_GPU_MEMORY_REQUEST` and `NVIDIA_GPU_MEMORY_LIMIT`, in MiB, from the container's env.
- It calls NVML `nvmlDeviceSetMemoryLimits_v1` on the container's cgroup: "For all products", Linux, cgroup v2, root (go-nvml `nvml.h:7320-7347`).
- The plugin's spec carries the hook (`full-gpu-nvml.go:176-187`).
- Not settled: whether graphics and NVENC allocations count, and whether 615 implements it on GeForce. E6b.

### 4.2 What a pod requests

- `resources.limits: {nvidia.com/gpu.shared: "1"}` with the rename, else `nvidia.com/gpu`.
- Extended resources go in limits; requests, if given, must equal them. (I high, Kubernetes rule)

### 4.3 Consumer limits

- NVIDIA's support matrix: GeForce RTX 3080, Ampere, 1 NVENC, 12 max concurrent sessions; RTX A5000 "Unrestricted" [S10]. (V, read 2026-11-08)
- The limit is the driver's, per GPU, so all desktops on the card share 12. (I high)
- One session per streaming client; Sunshine also opens short probe sessions when it starts. (I med)
- One NVENC engine: its throughput, not the session count, will cap first at 1080p60. (I med) E6 measures.
- 10 GB VRAM; each CUDA context costs some, and Sunshine opens one per stream. (I med)
- Start at 6 replicas; set the real number from E6.

## 5. Security cost

The owner's rule: least possible privilege. What each requirement costs.

### 5.1 Device nodes

- `nvidiactl`, `nvidia0`, `nvidia-uvm`: the driver's whole ioctl interface. A driver bug is a node compromise; the open modules do not shrink that interface. (I high)
- `nvidia-uvm-tools`: injected whatever we do (`common-nvml.go:57`). Profiling is admin only by default. (I med)
- `renderD`: render only, no modesetting by design. (I high)
- `cardN`, the primary node:
  - Sunshine 2026.914's CUDA path opens it read-write (2.1).
  - The first process to open it becomes DRM master when nobody holds it [S7]; master drives the card's outputs.
  - Sunshine drops master; any other program in the desktop may keep it.
  - No tenant scans out here, so master leaks no tenant's pixels. (I med)
  - The kernel's fbdev console (fbdev=1) could appear on a monitor plugged into the node. (I low)
- The open works because the spec adds the node's group, video, to the container's groups (`internal/edits/device.go:119-136`). (V)
- Later: `CDI_FEATURE_FLAGS=no-additional-gids-for-device-nodes` on the plugin (`cmd/nvidia-device-plugin/main.go:182-186`, `pkg/nvcdi/api.go:96-98`).
  Then cardN only exists, as gamescope needs, and only world read-write nodes open.
  Needs a stable Sunshine with #4946, and a world read-write renderD (E0).
- In a user namespace pod (`nestedSandboxes`) that added group maps to nothing on the node. (I high)
  So cardN does not open there, renderD only if world read-write.
  Sunshine 2026.914's NVENC path then fails in sandbox desktops; #4946 fixes it. (I med)
- The persistenced socket is mounted in; feature flag `disable-ipc-discoverer` drops it (`api.go:100-102`). (V)

### 5.2 Capabilities

- None added.
- Sunshine asks for `CAP_SYS_ADMIN` before the card open, logs the failure, and opens anyway (V, `kmsgrab.cpp:49-56, 94-100`).
- The images already strip file capabilities.

### 5.3 SELinux on MicroShift

Labels (V on Fedora 44 with matchpathcon; I high that CentOS Stream 10 matches):

- `/dev/nvidia*`: `xserver_misc_device_t`.
- `/dev/dri/*`: `dri_device_t`.

container-selinux, main branch read 2026-11-08 (V, `container.te`):

- `container_use_dri_devices`, default on, all container domains (:56-61, :1519-1521). The DRI nodes need nothing.
- `container_use_xserver_devices`, default off, `container_t` only (:49-54, :1514-1517). Added May 2024, PR #306.
- `container_use_devices`, default off, any device, all container domains (:44-47, :1509-1512).
- `container_device_t`: a type allowed any device node (:1644-1646).

So:

- A restricted-v2 desktop (`container_t`) is denied `/dev/nvidia*` today. (I high)
- The plugin works because it is privileged (`spc_t`).
- Sandbox desktops run as `container_engine_t` (docs/design.md, Order of work, item 3); the xserver boolean does not cover them. (V)
- `map` on those nodes was missing from the boolean once, fixed in container-selinux 2.218.0 [S15].

Options:

1. `setsebool -P container_use_xserver_devices on`.
   One switch, but `container_t` only.
2. A CIL module: `container_t` and `container_engine_t` may read, write, ioctl and map `xserver_misc_device_t` character devices. Recommended.
   Both are semodule transactions; an image built node installs them at build time, the way it installs its other policy modules.
3. `container_device_t` through a custom SCC: any device for that pod, a new SCC to own, and no room for `container_engine_t`. Rejected.
4. Relabel `/dev/nvidia*` to `container_file_t`, older GPU practice [S16]: every container type gets them. Rejected.

Cost of option 2: a `container_t` or `container_engine_t` process holding those nodes may use them. Only pods the device plugin served hold them. (I high)

### 5.4 Seccomp

- RuntimeDefault should do: the NVIDIA stack uses ioctl and mmap, nothing unusual. (I med)
- The hooks run outside the container's filter.
- E1 runs under RuntimeDefault; E7 under the sandbox profile.

### 5.5 Hooks

- Up to six createContainer hooks run as root on the node for each GPU container; E0 lists them. (V)
- They read the container's config and env. They join the trusted base, as the plugin already did. (I high)

## 6. Steam and gamescope

- gamescope stays nested in sway, the path proven on the Radeon.
- It needs a primary node to exist; the spec injects cardN. (V)
- It needs Vulkan with DRM format modifiers and `VK_EXT_physical_device_drm`; the NVIDIA driver has offered both for years. (I med)
- Xwayland inside gamescope: GLAMOR over NVIDIA's GBM [S1]; explicit sync since 555 [S5].
  gamescope's own explicit sync support was not checked.
- 32-bit: NVIDIA's EL10 repository has no i686 userspace, so neither has the node. (V)
  32-bit Proton games and any 32-bit part of Steam get no NVIDIA driver. (I med)
  Options: i686 libraries on the node from another source matching 615.71.09 exactly, or 64-bit games only.
- Steam's runtime takes the graphics driver from the container's /usr; CDI put it there. (I med)
- Sandbox desktops: option 2 in 5.3 covers `container_engine_t`; the card node is out of reach there (5.1).

## Plan

### Experiments on the node

Ordered smallest first, for any node as described above. This document touched no cluster.
Namespace: a scratch one, `deskube-gpu`.
Every pod as a desktop: non-root, all capabilities dropped, no privilege escalation, RuntimeDefault seccomp, unless the step says otherwise.

#### E0. Node facts, read only

From `oc debug node/<node>` then `chroot /host`, or ssh:

    cat /sys/module/nvidia_drm/parameters/modeset
    ls -lZ /dev/nvidia* /dev/dri /dev/dri/by-path
    ls /sys/class/drm; cat /sys/class/drm/card*/device/vendor
    getsebool container_use_dri_devices container_use_xserver_devices container_use_devices
    rpm -q container-selinux
    ls /var/run/cdi
    S=/var/run/cdi/k8s.device-plugin.nvidia.com-gpu.json
    jq '.devices[].containerEdits.deviceNodes, .containerEdits.deviceNodes' $S
    jq -r '.containerEdits.mounts[].containerPath' $S | grep -E 'EGL_nvidia|encode|allocator|egl-gbm|icd|egl_vendor|gbm.json'
    jq -r '.. | .hooks? // empty | .[].args[1]' $S | sort | uniq -c
    jq '.. | .additionalGids? // empty' $S
    oc get node -o jsonpath='{.items[0].status.allocatable}'

Expected:

- modeset `Y`.
- cardN and renderDN of the NVIDIA GPU; renderD world read-write is hoped, not known.
- The spec lists libnvidia-encode, libEGL_nvidia, libnvidia-allocator, egl-gbm, `10_nvidia.json`, the Vulkan ICD.
- Hooks include create-symlinks, update-ldcache, apply-cuda-memory-limits.
- `additionalGids` holds video's GID.
- No Intel vendor `0x8086` in `/sys/class/drm` (2.2).

Failure means:

- No DRM nodes in the spec: the plugin started before nvidia-drm. Restart its pod; then order it on the node.
- No graphics mounts: the host libraries are missing; fix the node's package list.
- An Intel card: the portal fallback copies through memory.

#### E1. nvidia-smi through CDI

    apiVersion: v1
    kind: Pod
    metadata: {name: smi, namespace: deskube-gpu}
    spec:
      restartPolicy: Never
      containers:
        - name: smi
          image: registry.fedoraproject.org/fedora-minimal:latest
          command: [sh, -c, 'nvidia-smi; id; ls -l /dev/nvidia* /dev/dri; sleep 600']
          resources: {limits: {nvidia.com/gpu: "1"}}
          securityContext:
            allowPrivilegeEscalation: false
            capabilities: {drop: [ALL]}
            runAsNonRoot: true
            seccompProfile: {type: RuntimeDefault}

Expected: the RTX 3080 and driver 615.71.09; `id` shows the video group.

Failure means:

- "Insufficient Permissions" or no device, with AVC denials (`ausearch -m avc -ts recent` on the node): SELinux, as 5.3 predicts.
  Confirm without persisting: `setsebool container_use_xserver_devices on` (no `-P`), rerun, set it back.
  That proves the node change before building it.
- Pod stuck Pending: the plugin advertises nothing.

E1b: the same pod under the nested-container SCC with `hostUsers: false`, as `nestedSandboxes` makes it.
Expected: works only with option 2 of 5.3, not with the boolean.
Also failure: a hook error at container create means the hooks do not run in a user namespace pod.

#### E2. Offscreen GPU rendering, no compositor

A throwaway Fedora image with vulkan-tools, vkmark and egl-utils, built by hand, not pushed.

    VK_DRIVER_FILES=/etc/vulkan/icd.d/nvidia_icd.x86_64.json vulkaninfo --summary
    vkmark --winsys headless
    eglinfo -B

Expected: the RTX 3080 in vulkaninfo; a vkmark score; NVIDIA for the GBM and device platforms in eglinfo.

Failure means:

- Vulkan works, EGL GBM does not: egl-gbm or `15_nvidia_gbm.json` missing from the spec.
- Neither: devices or SELinux, back to E1.

#### E3. sway headless on NVIDIA

The current desktop image, as a VdiInstance or a plain pod with a GPU limit, and:

    DESKUBE_RENDER_NODE=<renderD from E0>
    WLR_RENDERER=gles2
    __EGL_VENDOR_LIBRARY_FILENAMES=/usr/share/glvnd/egl_vendor.d/10_nvidia.json
    DESKUBE_ENCODER=software      # vaapi would fail; isolates the compositor

Then in the pod: `sway.log` names the NVIDIA EGL device or GBM; `vkcube` under `WAYLAND_DISPLAY`; `grim` and a pixel check, as the images README does.

Expected: sway up on NVIDIA's EGL; the cube drawn by the RTX 3080 (`nvidia-smi` on the node lists the process).

Failure means:

- "Failed to initialize EGL context": drop `__EGL_VENDOR_LIBRARY_FILENAMES` to see which vendor wins; try `WLR_RENDERER=vulkan`.
- GBM allocation errors: headless NVIDIA GBM fails for the compositor too. The design falls; stop and report.

#### E4. Sunshine on NVENC

E3's pod with `DESKUBE_ENCODER=nvenc DESKUBE_CAPTURE=wlr DESKUBE_LOG_LEVEL=debug`.

Expected in `sunshine.log`:

- the nvenc probe passes for H.264 and HEVC, not AV1;
- Wayland capture chosen;
- no "Falling back to GPU -> RAM -> GPU", no software encoder.

Failure means:

- "Couldn't open DRM FD for CUDA device": no video group (E0) or a user namespace. Rerun with Sunshine v2026.1107.
- "Failed to create GBM buffer": #4946's case. Rerun with v2026.1107; its shm fallback costs a copy.
- nvenc probe fails at once: libnvidia-encode or libcuda missing (E0).
- All of these: `capture = portal`, `encoder = vulkan`.

#### E5. A stream counted by the headless client

`hack/e2e-operator.sh` with a GPU limit (see Operator changes), a moving picture in the desktop (vkcube), `MIN_FPS=55`.
Runs: 1920x1080 at 60 H.264; 2560x1440 at 120 HEVC.
Sunshine's frame processing time from the debug log.

Expected: 59.5 and 119 fps delivered; processing under 5 ms on average (the Radeon did 2.2 and 3.4).

Failure means:

- fps short with an idle GPU: capture pacing, or the shm fallback.
- fps short with NVENC busy: lower `nvenc_twopass`, then the preset.

#### E6. Several desktops on one GPU

After the time slicing change, replicas 6:

- 1, 2, 4, 6 desktops, each with vkcube and a 1080p60 stream.
- Record per step: fps per stream, `nvidia-smi dmon -s u` (sm, enc), `nvidia-smi --query-gpu=memory.used --format=csv`.
- Then replicas 13 for one run: the 13th stream should be refused; note the error.

E6b, the memory limit hook: one desktop with `NVIDIA_GPU_MEMORY_LIMIT=1024`; allocate 2 GiB once from Vulkan, once from CUDA.

Expected: fps holds until enc nears 100 %; the 13th session refused; the limit stops CUDA (I low for Vulkan).

Failure means: fewer replicas; or the limit is useless for desktops and VRAM stays unbounded.

#### E7. Sandboxed Steam desktop

`deskube-steam`, `nestedSandboxes: true`, a GPU limit, after the SELinux module.

Expected: gamescope nested; `hack/gametest/check.sh` passes; Proton D3D11 on the RTX 3080.

Failure means:

- Hook errors at create: CDI hooks and user namespaces do not mix.
- "no primary node" from gamescope: cardN missing from the spec.
- 32-bit errors: section 6.
- GL games on llvmpipe: try `__GLX_VENDOR_LIBRARY_NAME=nvidia`.

### Node changes

After E1 confirms the SELinux denial.
How a node installs them is its own business; the content is below.

Time slicing, a ConfigMap next to the device plugin:

    # Desktops share the card. Nothing isolates them: no memory limit,
    # no fair share, one fault domain. renameByDefault makes that visible
    # in the resource name; a request for 2 would buy nothing.
    apiVersion: v1
    kind: ConfigMap
    metadata:
      name: nvidia-device-plugin-config
      namespace: nvidia-device-plugin
    data:
      config.yaml: |
        version: v1
        sharing:
          timeSlicing:
            renameByDefault: true
            failRequestsGreaterThanOne: true
            resources:
              - name: nvidia.com/gpu
                replicas: 6

The plugin's DaemonSet reads it:

                 - name: NVIDIA_DRIVER_CAPABILITIES
                   value: compute,utility
    +            # Read at start only: a change needs a restart of this pod.
    +            - name: CONFIG_FILE
    +              value: /etc/nvidia-device-plugin/config.yaml
     ...
               volumeMounts:
    +            - name: config
    +              mountPath: /etc/nvidia-device-plugin
    +              readOnly: true
     ...
           volumes:
    +        - name: config
    +          configMap:
    +            name: nvidia-device-plugin-config

Cost: anything asking `nvidia.com/gpu` stops scheduling.
The Helm chart of the plugin takes the same file through its `config` values.

SELinux, a CIL module, `container-nvidia.cil`:

    ; /dev/nvidia* are xserver_misc_device_t. container-selinux grants them
    ; only through booleans, and container_use_xserver_devices covers
    ; container_t alone; DeskuBe's sandbox desktops run container_engine_t.
    ; The device cgroup still limits these to pods the plugin served.
    (allow container_t xserver_misc_device_t (chr_file (getattr open read write append ioctl lock map)))
    (allow container_engine_t xserver_misc_device_t (chr_file (getattr open read write append ioctl lock map)))

On a mutable node: `semodule -i container-nvidia.cil`.
On an image built node: install it in the image build, with its other policy modules.

CDI capabilities: no change (3.4).
`NVIDIA_DRIVER_CAPABILITIES` on the plugin is inert; leave it or drop it, no effect either way.

Later, after a stable Sunshine with #4946 and a world read-write renderD:

    +            # Only world read-write nodes open; the card node only exists.
    +            - name: CDI_FEATURE_FLAGS
    +              value: no-additional-gids-for-device-nodes

### Image changes

One image. No variant.

- NVIDIA userspace comes from the node through CDI and must match its kernel module, so the image carries none. (I high)
- Mesa stays for AMD, Intel and the CPU path; two variables keep it out of NVIDIA's way (3.5).

`base/rootfs/usr/local/bin/desk-session`, proposed:

    -RENDER=${DESKUBE_RENDER_NODE:-/dev/dri/renderD128}
    +# A device plugin hands over its GPU's own render node, renderD128
    +# only when that GPU is the node's first.
    +RENDER=${DESKUBE_RENDER_NODE:-$(ls /dev/dri/renderD* 2> /dev/null | head -1)}
     ...
     if [ -e "$RENDER" ]; then
       export WLR_RENDER_DRM_DEVICE=$RENDER
    -  ENCODER=${DESKUBE_ENCODER:-vaapi}
    +  GPU_ENCODER=vaapi
    +  if [ "$(basename "$(readlink -f "/sys/class/drm/${RENDER##*/}/device/driver")")" = nvidia ]; then
    +    # glvnd and the Vulkan loader also offer Mesa's drivers, and wlroots
    +    # gives up if the first EGL device matching the node fails.
    +    export WLR_RENDERER=gles2
    +    f=/usr/share/glvnd/egl_vendor.d/10_nvidia.json
    +    [ -e "$f" ] && export __EGL_VENDOR_LIBRARY_FILENAMES=$f
    +    for f in /etc/vulkan/icd.d/nvidia_icd*.json; do
    +      [ -e "$f" ] && export VK_DRIVER_FILES=$f
    +    done
    +    GPU_ENCODER=nvenc
    +  fi
    +  ENCODER=${DESKUBE_ENCODER:-$GPU_ENCODER}
       CAPTURE=${DESKUBE_CAPTURE:-wlr}

- `adapter_name = $RENDER` already follows.
- `base/pins`: move Sunshine to the first stable release with #4946 if E4 needs it.
- Steam image: nothing new for 64-bit; 32-bit stays open (section 6).

### Operator changes

- GPU request: none needed in code.
  `spec.resources.limits: {nvidia.com/gpu.shared: "1"}` already reaches the container (`vdiinstance_controller.go:508`).
  The admission policy already keeps owners off `spec.resources`.
- A class field: not now. It belongs to "Pools and templates" (docs/design.md, Order of work, item 5), so admins stop repeating the resource name.
- `runtimeClassName`: none. CRI-O injects CDI itself.
- SELinux: nothing in the pod with option 2 of 5.3; restricted-v2 and nested-container stay as they are.
- Encoder, `api/v1alpha1/vdiinstance_types.go:70-72`:

      -	// Encoder is chosen by the image when empty: vaapi when the desktop
      -	// has a GPU, software otherwise.
      -	// +kubebuilder:validation:Enum=software;vaapi
      +	// Encoder is chosen by the image when empty: nvenc on an NVIDIA GPU,
      +	// vaapi on another GPU, software without one.
      +	// +kubebuilder:validation:Enum=software;vaapi;nvenc

  Then `make manifests`; `docs/design.md:51` lists the values.
- `hack/e2e-operator.sh`: a `GPU` variable naming the resource, written into `resources.limits`.
- Later, if E6b works: a per desktop GPU memory limit, passed as `NVIDIA_GPU_MEMORY_LIMIT`.
- README, Known gaps: say NVIDIA needs the node changes above.

## Biggest risks

1. Sunshine on headless NVIDIA.
   GBM capture buffers have failed for others, and our pinned Sunshine needs the card node.
   The fix is only in a pre-release; its fallback copies through memory.
2. SELinux on the node.
   `container_t` and `container_engine_t` cannot use `/dev/nvidia*` today.
   The fix is a policy module on every GPU node, outside DeskuBe's reach.
3. Sharing without isolation.
   No VRAM limit, no fair share, one fault domain, 10 GB and one NVENC for every desktop on the card.
   One game can starve or crash the others.

Also: no 32-bit NVIDIA userspace for Steam; DRM master through the card node.

## Not settled

- Whether NVML's cgroup memory limit counts graphics and NVENC memory, and works on GeForce at 615. E6b.
- The NVENC throughput of one RTX 3080 for N streams at 1080p60. E6.
- The mode of renderD on CentOS Stream CoreOS 10. E0.
- Whether the node has another GPU, an Intel one especially. E0.
- Whether wlroots headless and Sunshine see torn or stale frames on NVIDIA without explicit sync on the headless backend. E5, with a moving picture.
- gamescope nested on NVIDIA in a pod; Xwayland's GLX vendor there. E7.
- Whether the CDI hooks run in a user namespace pod. E1b.
- The sway version Fedora 45 ships. Only matters below 1.10.

## Sources

Repositories and packages:

- desku.be-images at 08e89ef: `README.md`, `base/Containerfile`, `base/pins`, `base/rootfs/usr/local/bin/desk-session`, `steam/`.
- NVIDIA EL10 CUDA repository, read 2026-11-08: https://developer.download.nvidia.com/compute/cuda/repos/rhel10/x86_64/
  (filelists; RPMs nvidia-kmod-common-615.71.09, nvidia-driver-selinux-0.1-3, nvidia-container-toolkit-base-1.20.1).

Source trees, files cited inline:

- https://github.com/NVIDIA/nvidia-container-toolkit/tree/v1.20.1
- https://github.com/NVIDIA/k8s-device-plugin/tree/v0.20.1
- https://github.com/LizardByte/Sunshine/tree/v2026.914.233613
- https://github.com/swaywm/sway (1.10 and master)
- https://gitlab.freedesktop.org/wlroots/wlroots (master)
- https://gitlab.freedesktop.org/mesa/mesa/-/blob/main/src/gbm/main/backend.c
- https://github.com/containers/container-selinux/blob/main/container.te

Numbered:

- [S1] NVIDIA README 550.67, GBM: https://download.nvidia.com/XFree86/Linux-x86_64/550.67/README/gbm.html
  GBM first appears in 495.44: https://download.nvidia.com/XFree86/Linux-x86_64/495.44/README/gbm.html
- [S2] https://github.com/NVIDIA/open-gpu-kernel-modules/blob/615.71.09/kernel-open/nvidia-drm/nvidia-drm-os-interface.c
- [S3] https://github.com/daaaaan/sunshine-headless-sway
- [S4] Sunshine PR #4946: https://github.com/LizardByte/Sunshine/pull/4946
- [S5] NVIDIA 555 explicit sync: https://www.phoronix.com/news/NVIDIA-555.42.02-Linux-Beta
- [S6] gamescope issue #1984: https://github.com/ValveSoftware/gamescope/issues/1984
- [S7] Sunshine PR #5286: https://github.com/LizardByte/Sunshine/pull/5286
- [S8] Sunshine PR #4603, the vulkan encoder: https://github.com/LizardByte/Sunshine/pull/4603
- [S9] Sunshine PR #5030: https://github.com/LizardByte/Sunshine/pull/5030
- [S10] NVIDIA Video Encode and Decode GPU Support Matrix: https://developer.nvidia.com/video-encode-and-decode-gpu-support-matrix-new
- [S11] Kubernetes removed feature gates, DevicePluginCDIDevices: https://kubernetes.io/docs/reference/command-line-tools-reference/feature-gates-removed/
- [S12] crio.conf(5), cdi_spec_dirs: https://github.com/cri-o/cri-o/blob/main/docs/crio.conf.5.md
- [S13] libglvnd EGL vendor enumeration: https://github.com/NVIDIA/libglvnd/blob/master/src/EGL/icd_enumeration.md
- [S14] Vulkan loader, VK_DRIVER_FILES: https://github.com/KhronosGroup/Vulkan-Loader/blob/main/docs/LoaderInterfaceArchitecture.md
- [S15] Red Hat bug 2214569, map on NVIDIA devices: https://bugzilla.redhat.com/show_bug.cgi?id=2214569
- [S16] IBM, relabelling /dev/nvidia*: https://www.ibm.com/docs/en/SSQNUZ_3.5.0/wsj/admin/gpu-node-settings.html
