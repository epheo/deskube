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
import { durationText, sizeText } from '../format';
import { useVdiClasses } from '../hooks/useVdiClasses';
import { VdiInstance, VdiInstanceSpec } from '../types';

interface Props {
  namespace?: string;
  onClose: () => void;
}

const IDLE_OPTIONS = [
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
  const [classes, classesLoaded, classesError] = useVdiClasses();

  const [name, setName] = useState('');
  const [namespace, setNamespace] = useState(activeNamespace ?? 'default');
  // Unset until the user picks, so each follows the class until then.
  const [classChoice, setClassChoice] = useState<string>();
  const [imageChoice, setImageChoice] = useState<string>();
  const [sizeChoice, setSizeChoice] = useState<string>();
  const [owner, setOwner] = useState('');
  const [idleTimeout, setIdleTimeout] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  const className = (classChoice ?? classes[0]?.metadata?.name ?? '').trim();
  const desktopClass = classes.find((c) => c.metadata?.name === className);
  const image = (imageChoice ?? (className ? '' : DEFAULT_IMAGE)).trim();
  const size = className ? sizeChoice ?? '' : sizeChoice || 'small';

  const valid = /^[a-z0-9]([-a-z0-9]*[a-z0-9])?$/.test(name) && namespace !== '' && (className !== '' || image !== '');

  const submit = async () => {
    setError(null);
    setSubmitting(true);
    const spec: VdiInstanceSpec = {
      ...(className ? { className } : {}),
      ...(image ? { image } : {}),
      ...(size
        ? {
            resources: {
              requests: { cpu: SIZES[size].cpu, memory: SIZES[size].memory },
              limits: { memory: SIZES[size].memory },
            },
          }
        : {}),
      ...(owner.trim() ? { owner: owner.trim() } : {}),
      ...(idleTimeout ? { idleTimeout } : {}),
    };
    const desktop: VdiInstance = {
      apiVersion: 'vdi.desku.be/v1alpha1',
      kind: 'VdiInstance',
      metadata: { name, namespace },
      spec,
    };
    try {
      await k8sCreate<VdiInstance>({ model, data: desktop });
      onClose();
    } catch (e) {
      setError(String((e as Error)?.message ?? e));
      setSubmitting(false);
    }
  };

  const classIdle = desktopClass?.spec.idleTimeout;
  const fromClass = (what?: string) => (desktopClass ? `From the class: ${what}` : 'From the class');

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
          <FormGroup label="Class" fieldId="class">
            {classesError ? (
              <TextInput id="class" value={classChoice ?? ''} onChange={(_e, v) => setClassChoice(v)} placeholder="desktop" />
            ) : (
              <FormSelect id="class" value={className} onChange={(_e, v) => setClassChoice(v)} isDisabled={!classesLoaded}>
                {classes.map((c) => (
                  <FormSelectOption key={c.metadata?.name} value={c.metadata?.name} label={c.metadata?.name ?? ''} />
                ))}
                <FormSelectOption value="" label="None, give an image" />
              </FormSelect>
            )}
            <HelperText>
              <HelperTextItem>
                {classesError
                  ? 'Cannot list desktop classes. Type a class name, or leave it empty and give an image.'
                  : classesLoaded && classes.length === 0
                    ? 'No desktop classes yet: give an image.'
                    : className
                      ? 'What you set below overrides the class.'
                      : 'The desktop takes only what you set below.'}
              </HelperTextItem>
            </HelperText>
          </FormGroup>
          <FormGroup label="Image" isRequired={!className} fieldId="image">
            <TextInput
              id="image"
              value={imageChoice ?? image}
              onChange={(_e, v) => setImageChoice(v)}
              placeholder={desktopClass?.spec.image}
            />
            <HelperText>
              <HelperTextItem>
                {className ? "Optional. Leave empty for the class's image." : 'Container image of the desktop.'}
              </HelperTextItem>
            </HelperText>
          </FormGroup>
          <FormGroup label="Size" fieldId="size">
            <FormSelect id="size" value={size} onChange={(_e, v) => setSizeChoice(v)}>
              {className && <FormSelectOption value="" label={fromClass(sizeText(desktopClass?.spec.resources) ?? 'none set')} />}
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
              <FormSelectOption
                value=""
                label={className ? fromClass(classIdle ? durationText(classIdle) : 'never') : 'Never'}
              />
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
