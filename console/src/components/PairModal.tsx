import { useMemo, useState } from 'react';
import type { FC } from 'react';
import {
  Alert,
  Button,
  ClipboardCopy,
  ClipboardCopyVariant,
  Content,
  Form,
  FormGroup,
  HelperText,
  HelperTextItem,
  Modal,
  ModalBody,
  ModalFooter,
  ModalHeader,
  ModalVariant,
  Spinner,
  TextInput,
} from '@patternfly/react-core';
import { k8sPatch, useK8sModel, useK8sWatchResource } from '@openshift-console/dynamic-plugin-sdk';
import { VdiInstanceGVK } from '../constants';
import { VdiInstance } from '../types';

interface Props {
  desktop: VdiInstance;
  onClose: () => void;
}

// The PIN never reaches the guest from here. The operator hands it to
// Sunshine, so the owner only needs patch on their own desktop.
export const PairModal: FC<Props> = ({ desktop, onClose }) => {
  const [model] = useK8sModel(VdiInstanceGVK);
  const [pin, setPin] = useState('');
  const [clientName, setClientName] = useState('');
  // A PIN still on the desktop is followed, not asked for again.
  const [sentPin, setSentPin] = useState<string | null>(desktop.spec.pairing?.pin ?? null);
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  const namespace = desktop.metadata?.namespace ?? '';
  const instance = desktop.metadata?.name ?? '';

  const watch = useMemo(() => ({ groupVersionKind: VdiInstanceGVK, name: instance, namespace }), [instance, namespace]);
  const [live] = useK8sWatchResource<VdiInstance>(watch);
  // status.pairing names its PIN, so an earlier outcome never answers this one.
  const pairing = sentPin && live?.status?.pairing?.pin === sentPin ? live.status.pairing : undefined;
  const phase = sentPin ? pairing?.phase ?? 'Pending' : undefined;

  const pinValid = /^[0-9]{4}$/.test(pin);

  const submit = async () => {
    setError(null);
    setSubmitting(true);
    try {
      await k8sPatch<VdiInstance>({
        model,
        resource: desktop,
        data: [{ op: 'add', path: '/spec/pairing', value: { pin, clientName: clientName.trim() || 'moonlight' } }],
      });
      setSentPin(pin);
    } catch (e) {
      setError(String((e as Error)?.message ?? e));
    }
    setSubmitting(false);
  };

  const reset = () => {
    setSentPin(null);
    setPin('');
  };

  return (
    <Modal variant={ModalVariant.small} isOpen onClose={onClose} aria-label="Pair a Moonlight client">
      <ModalHeader title={`Pair with ${instance}`} />
      <ModalBody>
        <Content component="p">In Moonlight, add this host. Moonlight then shows a 4 digit PIN.</Content>
        <ClipboardCopy variant={ClipboardCopyVariant.inlineCompact} isReadOnly hoverTip="Copy" clickTip="Copied">
          {desktop.status?.address ?? ''}
        </ClipboardCopy>
        {!sentPin ? (
          <Form style={{ marginTop: 16 }} onSubmit={(e) => { e.preventDefault(); if (pinValid) submit(); }}>
            <FormGroup label="PIN" isRequired fieldId="pin">
              <TextInput id="pin" value={pin} onChange={(_e, v) => setPin(v.replace(/\D/g, '').slice(0, 4))} placeholder="1234" />
              <HelperText>
                <HelperTextItem>Four digits, exactly as Moonlight shows them.</HelperTextItem>
              </HelperText>
            </FormGroup>
            <FormGroup label="Client name" fieldId="client">
              <TextInput id="client" value={clientName} onChange={(_e, v) => setClientName(v)} placeholder="laptop" />
            </FormGroup>
            {error && <Alert variant="danger" isInline title={error} />}
          </Form>
        ) : phase === 'Paired' ? (
          <Alert variant="success" isInline title="Paired" style={{ marginTop: 16 }}>
            {pairing?.message}. Moonlight now lists Desktop.
          </Alert>
        ) : phase === 'Failed' ? (
          <Alert variant="danger" isInline title="Pairing failed" style={{ marginTop: 16 }}>
            {pairing?.message}
          </Alert>
        ) : (
          <Alert variant="info" isInline title="Waiting for Sunshine" style={{ marginTop: 16 }} customIcon={<Spinner size="md" />}>
            {pairing?.message ?? 'Sending the PIN to the desktop.'}
          </Alert>
        )}
      </ModalBody>
      <ModalFooter>
        {!sentPin ? (
          <Button variant="primary" onClick={submit} isDisabled={!pinValid || submitting} isLoading={submitting}>
            Pair
          </Button>
        ) : phase === 'Failed' ? (
          <Button variant="primary" onClick={reset}>
            Try again
          </Button>
        ) : phase === 'Pending' ? (
          <Button variant="secondary" onClick={reset}>
            Use another PIN
          </Button>
        ) : null}
        <Button variant="link" onClick={onClose}>
          Close
        </Button>
      </ModalFooter>
    </Modal>
  );
};
