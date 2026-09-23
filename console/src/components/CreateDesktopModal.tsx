import { useMemo, useState } from 'react';
import type { FC } from 'react';
import {
  Alert,
  Button,
  Form,
  FormGroup,
  FormSelect,
  FormSelectOption,
  HelperText,
  HelperTextItem,
  Modal,
  ModalBody,
  ModalFooter,
  ModalHeader,
  ModalVariant,
  TextInput,
} from '@patternfly/react-core';
import {
  K8sResourceCommon,
  k8sCreate,
  useK8sModel,
  useK8sWatchResource,
} from '@openshift-console/dynamic-plugin-sdk';
import { ClusterInstancetypeGVK, DataSourceGVK, VdiInstanceGVK } from '../constants';
import { VdiInstance } from '../types';

interface Props {
  namespace?: string;
  onClose: () => void;
}

const IDLE_OPTIONS = [
  { value: '', label: 'Never' },
  { value: '30m', label: '30 minutes' },
  { value: '1h', label: '1 hour' },
  { value: '4h', label: '4 hours' },
  { value: '8h', label: '8 hours' },
];

const nameOf = (r: K8sResourceCommon): string => r.metadata?.name ?? '';

export const CreateDesktopModal: FC<Props> = ({ namespace: activeNamespace, onClose }) => {
  const [model] = useK8sModel(VdiInstanceGVK);
  const [name, setName] = useState('');
  const [namespace, setNamespace] = useState(activeNamespace ?? 'default');
  const [image, setImage] = useState('');
  const [instanceType, setInstanceType] = useState('u1.xlarge');
  const [owner, setOwner] = useState('');
  const [idleTimeout, setIdleTimeout] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  // Both lists are optional: a plain owner may not be allowed to read them.
  const imagesWatch = useMemo(
    () => ({ groupVersionKind: DataSourceGVK, isList: true, namespace, optional: true }),
    [namespace],
  );
  const [images] = useK8sWatchResource<K8sResourceCommon[]>(imagesWatch);
  const typesWatch = useMemo(() => ({ groupVersionKind: ClusterInstancetypeGVK, isList: true, optional: true }), []);
  const [types] = useK8sWatchResource<K8sResourceCommon[]>(typesWatch);

  const imageNames = (images ?? []).map(nameOf).sort();
  const typeNames = (types ?? []).map(nameOf).filter((n) => n.startsWith('u1.')).sort();
  if (!image && imageNames.length > 0) {
    setImage(imageNames[0]);
  }

  const valid = /^[a-z0-9]([-a-z0-9]*[a-z0-9])?$/.test(name) && namespace !== '' && image !== '';

  const submit = async () => {
    setError(null);
    setSubmitting(true);
    const desktop: VdiInstance = {
      apiVersion: 'vdi.desku.be/v1alpha1',
      kind: 'VdiInstance',
      metadata: { name, namespace },
      spec: {
        image: { dataSource: { name: image } },
        instanceType,
        ...(owner.trim() ? { owner: owner.trim() } : {}),
        ...(idleTimeout ? { idleTimeout } : {}),
      },
    };
    try {
      await k8sCreate<VdiInstance>({ model, data: desktop });
      onClose();
    } catch (e) {
      setError(String((e as Error)?.message ?? e));
      setSubmitting(false);
    }
  };

  return (
    <Modal variant={ModalVariant.medium} isOpen onClose={onClose} aria-label="Create desktop">
      <ModalHeader title="Create desktop" />
      <ModalBody>
        <Form id="create-desktop" onSubmit={(e) => { e.preventDefault(); if (valid) submit(); }}>
          <FormGroup label="Name" isRequired fieldId="name">
            <TextInput id="name" value={name} onChange={(_e, v) => setName(v)} placeholder="alice-desk" />
          </FormGroup>
          {!activeNamespace && (
            <FormGroup label="Namespace" isRequired fieldId="namespace">
              <TextInput id="namespace" value={namespace} onChange={(_e, v) => setNamespace(v)} />
            </FormGroup>
          )}
          <FormGroup label="Golden image" isRequired fieldId="image">
            {imageNames.length > 0 ? (
              <FormSelect id="image" value={image} onChange={(_e, v) => setImage(v)}>
                {imageNames.map((n) => (
                  <FormSelectOption key={n} value={n} label={n} />
                ))}
              </FormSelect>
            ) : (
              <TextInput id="image" value={image} onChange={(_e, v) => setImage(v)} placeholder="deskube-fedora-xfce" />
            )}
            <HelperText>
              <HelperTextItem>A CDI DataSource in the same namespace.</HelperTextItem>
            </HelperText>
          </FormGroup>
          <FormGroup label="Size" fieldId="instanceType">
            {typeNames.length > 0 ? (
              <FormSelect id="instanceType" value={instanceType} onChange={(_e, v) => setInstanceType(v)}>
                {typeNames.map((n) => (
                  <FormSelectOption key={n} value={n} label={n} />
                ))}
              </FormSelect>
            ) : (
              <TextInput id="instanceType" value={instanceType} onChange={(_e, v) => setInstanceType(v)} />
            )}
          </FormGroup>
          <FormGroup label="Owner" fieldId="owner">
            <TextInput id="owner" value={owner} onChange={(_e, v) => setOwner(v)} placeholder="alice" />
            <HelperText>
              <HelperTextItem>Kubernetes user who may pair clients and start or stop this desktop.</HelperTextItem>
            </HelperText>
          </FormGroup>
          <FormGroup label="Stop when idle for" fieldId="idle">
            <FormSelect id="idle" value={idleTimeout} onChange={(_e, v) => setIdleTimeout(v)}>
              {IDLE_OPTIONS.map((o) => (
                <FormSelectOption key={o.value} value={o.value} label={o.label} />
              ))}
            </FormSelect>
          </FormGroup>
          {error && <Alert variant="danger" isInline title={error} />}
        </Form>
      </ModalBody>
      <ModalFooter>
        <Button variant="primary" form="create-desktop" onClick={submit} isDisabled={!valid || submitting} isLoading={submitting}>
          Create
        </Button>
        <Button variant="link" onClick={onClose}>
          Cancel
        </Button>
      </ModalFooter>
    </Modal>
  );
};
