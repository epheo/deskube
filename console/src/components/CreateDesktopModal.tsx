import { useState } from 'react';
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
import { k8sCreate, useK8sModel } from '@openshift-console/dynamic-plugin-sdk';
import { DEFAULT_IMAGE, VdiInstanceGVK } from '../constants';
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

// Memory is requested and capped, CPU only requested: a CPU cap
// throttles, and a throttled desktop stutters.
const SIZES: Record<string, { label: string; cpu: string; memory: string }> = {
  small: { label: 'Small: 2 CPU, 4 GiB', cpu: '2', memory: '4Gi' },
  medium: { label: 'Medium: 4 CPU, 8 GiB', cpu: '4', memory: '8Gi' },
  large: { label: 'Large: 8 CPU, 16 GiB', cpu: '8', memory: '16Gi' },
};

export const CreateDesktopModal: FC<Props> = ({ namespace: activeNamespace, onClose }) => {
  const [model] = useK8sModel(VdiInstanceGVK);
  const [name, setName] = useState('');
  const [namespace, setNamespace] = useState(activeNamespace ?? 'default');
  const [image, setImage] = useState(DEFAULT_IMAGE);
  const [size, setSize] = useState('small');
  const [owner, setOwner] = useState('');
  const [idleTimeout, setIdleTimeout] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  const valid = /^[a-z0-9]([-a-z0-9]*[a-z0-9])?$/.test(name) && namespace !== '' && image.trim() !== '';

  const submit = async () => {
    setError(null);
    setSubmitting(true);
    const desktop: VdiInstance = {
      apiVersion: 'vdi.desku.be/v1alpha1',
      kind: 'VdiInstance',
      metadata: { name, namespace },
      spec: {
        image: image.trim(),
        resources: {
          requests: { cpu: SIZES[size].cpu, memory: SIZES[size].memory },
          limits: { memory: SIZES[size].memory },
        },
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
          <FormGroup label="Image" isRequired fieldId="image">
            <TextInput id="image" value={image} onChange={(_e, v) => setImage(v)} />
            <HelperText>
              <HelperTextItem>Container image of the desktop.</HelperTextItem>
            </HelperText>
          </FormGroup>
          <FormGroup label="Size" fieldId="size">
            <FormSelect id="size" value={size} onChange={(_e, v) => setSize(v)}>
              {Object.entries(SIZES).map(([key, s]) => (
                <FormSelectOption key={key} value={key} label={s.label} />
              ))}
            </FormSelect>
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
