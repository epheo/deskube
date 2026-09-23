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
import { k8sCreate, useK8sModel, useK8sWatchResource } from '@openshift-console/dynamic-plugin-sdk';
import { VdiPairingGVK } from '../constants';
import { VdiInstance, VdiPairing } from '../types';

interface Props {
  desktop: VdiInstance;
  onClose: () => void;
}

// The PIN never reaches the guest from here. The operator hands it to
// Sunshine, so the user only needs create on VdiPairing.
export const PairModal: FC<Props> = ({ desktop, onClose }) => {
  const [model] = useK8sModel(VdiPairingGVK);
  const [pin, setPin] = useState('');
  const [clientName, setClientName] = useState('');
  const [pairingName, setPairingName] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  const namespace = desktop.metadata?.namespace ?? '';
  const instance = desktop.metadata?.name ?? '';

  const watch = useMemo(
    () => (pairingName ? { groupVersionKind: VdiPairingGVK, name: pairingName, namespace } : null),
    [pairingName, namespace],
  );
  const [pairing] = useK8sWatchResource<VdiPairing>(watch);
  const phase = pairingName ? pairing?.status?.phase ?? 'Pending' : undefined;

  const pinValid = /^[0-9]{4}$/.test(pin);

  const submit = async () => {
    setError(null);
    setSubmitting(true);
    const name = `${instance}-${Date.now().toString(36)}`;
    try {
      await k8sCreate<VdiPairing>({
        model,
        data: {
          apiVersion: 'vdi.desku.be/v1alpha1',
          kind: 'VdiPairing',
          metadata: { name, namespace },
          spec: { instanceRef: instance, pin, clientName: clientName.trim() || 'moonlight' },
        },
      });
      setPairingName(name);
    } catch (e) {
      setError(String((e as Error)?.message ?? e));
    }
    setSubmitting(false);
  };

  const reset = () => {
    setPairingName(null);
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
        {!pairingName ? (
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
            {pairing?.status?.message}. Moonlight now lists Desktop.
          </Alert>
        ) : phase === 'Failed' ? (
          <Alert variant="danger" isInline title="Pairing failed" style={{ marginTop: 16 }}>
            {pairing?.status?.message}
          </Alert>
        ) : (
          <Alert variant="info" isInline title="Waiting for Sunshine" style={{ marginTop: 16 }} customIcon={<Spinner size="md" />}>
            {pairing?.status?.message ?? 'Sending the PIN to the desktop.'}
          </Alert>
        )}
      </ModalBody>
      <ModalFooter>
        {!pairingName ? (
          <Button variant="primary" onClick={submit} isDisabled={!pinValid || submitting} isLoading={submitting}>
            Pair
          </Button>
        ) : phase === 'Failed' ? (
          <Button variant="primary" onClick={reset}>
            Try again
          </Button>
        ) : null}
        <Button variant="link" onClick={onClose}>
          Close
        </Button>
      </ModalFooter>
    </Modal>
  );
};
