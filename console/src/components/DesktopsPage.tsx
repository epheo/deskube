import { useMemo, useState } from 'react';
import type { FC } from 'react';
import {
  Alert,
  Button,
  ClipboardCopy,
  ClipboardCopyVariant,
  EmptyState,
  EmptyStateBody,
  Label,
  LabelGroup,
  Modal,
  ModalBody,
  ModalFooter,
  ModalHeader,
  ModalVariant,
  PageSection,
  Spinner,
  Title,
  Toolbar,
  ToolbarContent,
  ToolbarItem,
  Tooltip,
} from '@patternfly/react-core';
import { ActionsColumn, IAction, Table, Tbody, Td, Th, Thead, Tr } from '@patternfly/react-table';
import {
  k8sDelete,
  k8sPatch,
  NamespaceBar,
  ResourceLink,
  Timestamp,
  useActiveNamespace,
  useK8sModel,
  useK8sWatchResource,
} from '@openshift-console/dynamic-plugin-sdk';
import {
  ALL_NAMESPACES,
  CONDITION_READY,
  CONDITION_UPDATE_PENDING,
  REASON_CLASS_NOT_FOUND,
  VdiInstanceGVK,
} from '../constants';
import { useNamespaceSync } from '../hooks/useNamespaceSync';
import { Condition, VdiInstance } from '../types';
import { CreateDesktopModal } from './CreateDesktopModal';
import { PairModal } from './PairModal';

type LabelColor = 'green' | 'blue' | 'grey' | 'red' | 'orange';

const phaseColor = (phase?: string): LabelColor => {
  switch (phase) {
    case 'Ready':
      return 'green';
    case 'Failed':
      return 'red';
    case 'Stopped':
      return 'grey';
    case 'Stopping':
      return 'orange';
    default:
      return 'blue';
  }
};

// The Sunshine web UI sits one port above the base port.
export const webUiUrl = (address?: string): string | undefined => {
  if (!address) return undefined;
  const i = address.lastIndexOf(':');
  if (i < 0) return undefined;
  return `https://${address.slice(0, i)}:${Number(address.slice(i + 1)) + 1}`;
};

const isRunning = (vdi: VdiInstance): boolean => vdi.spec.running !== false;

const condition = (vdi: VdiInstance, type: string): Condition | undefined =>
  vdi.status?.conditions?.find((c) => c.type === type);

const DesktopStatus: FC<{ vdi: VdiInstance }> = ({ vdi }) => {
  const classMissing = condition(vdi, CONDITION_READY)?.reason === REASON_CLASS_NOT_FOUND;
  const update = condition(vdi, CONDITION_UPDATE_PENDING);
  const request = vdi.spec.pairing;
  // An outcome for another PIN says nothing about the one waiting.
  const outcome = request && vdi.status?.pairing?.pin !== request.pin ? undefined : vdi.status?.pairing;
  const pairing = outcome?.phase ?? (request ? 'Pending' : undefined);
  return (
    <LabelGroup numLabels={4}>
      <Label color={phaseColor(vdi.status?.phase)}>{vdi.status?.phase ?? 'Pending'}</Label>
      {classMissing && (
        <Label color="red" variant="outline">
          Class {vdi.spec.className} does not exist
        </Label>
      )}
      {update?.status === 'True' && (
        <Tooltip content={update.message}>
          <Label color="orange" variant="outline">
            Update waits for the session to end
          </Label>
        </Tooltip>
      )}
      {pairing === 'Pending' && (
        <Tooltip content={outcome?.message ?? 'The PIN has not reached the desktop yet.'}>
          <Label color="blue" variant="outline">
            Pairing
          </Label>
        </Tooltip>
      )}
      {pairing === 'Failed' && (
        <Tooltip content={outcome?.message}>
          <Label color="red" variant="outline">
            Pairing failed
          </Label>
        </Tooltip>
      )}
    </LabelGroup>
  );
};

const DesktopsPage: FC = () => {
  useNamespaceSync();
  const [activeNamespace] = useActiveNamespace();
  const namespace = activeNamespace === ALL_NAMESPACES ? undefined : activeNamespace;
  const [model] = useK8sModel(VdiInstanceGVK);

  const watch = useMemo(
    () => ({ groupVersionKind: VdiInstanceGVK, isList: true, ...(namespace ? { namespace } : {}) }),
    [namespace],
  );
  const [desktops, loaded, loadError] = useK8sWatchResource<VdiInstance[]>(watch);

  const [pairing, setPairing] = useState<VdiInstance | null>(null);
  const [deleting, setDeleting] = useState<VdiInstance | null>(null);
  const [creating, setCreating] = useState(false);
  const [actionError, setActionError] = useState<string | null>(null);

  const setRunning = async (vdi: VdiInstance, running: boolean) => {
    setActionError(null);
    try {
      await k8sPatch({ model, resource: vdi, data: [{ op: 'add', path: '/spec/running', value: running }] });
    } catch (e) {
      setActionError(String((e as Error)?.message ?? e));
    }
  };

  const confirmDelete = async () => {
    if (!deleting) return;
    setActionError(null);
    try {
      await k8sDelete({ model, resource: deleting });
    } catch (e) {
      setActionError(String((e as Error)?.message ?? e));
    }
    setDeleting(null);
  };

  const actions = (vdi: VdiInstance): IAction[] => {
    const ready = vdi.status?.phase === 'Ready';
    const web = webUiUrl(vdi.status?.address);
    return [
      { title: 'Pair a Moonlight client', onClick: () => setPairing(vdi), isDisabled: !ready },
      isRunning(vdi)
        ? { title: 'Stop', onClick: () => setRunning(vdi, false) }
        : { title: 'Start', onClick: () => setRunning(vdi, true) },
      { title: 'Open Sunshine web UI', onClick: () => window.open(web, '_blank', 'noopener'), isDisabled: !web || !ready },
      { isSeparator: true },
      { title: 'Delete', onClick: () => setDeleting(vdi) },
    ];
  };

  const rows = [...(desktops ?? [])].sort((a, b) => (a.metadata?.name ?? '').localeCompare(b.metadata?.name ?? ''));

  return (
    <>
      <NamespaceBar />
      <PageSection>
        <Toolbar>
          <ToolbarContent>
            <ToolbarItem>
              <Title headingLevel="h1">Virtual Desktops</Title>
            </ToolbarItem>
            <ToolbarItem align={{ default: 'alignEnd' }}>
              <Button variant="primary" onClick={() => setCreating(true)}>
                Create desktop
              </Button>
            </ToolbarItem>
          </ToolbarContent>
        </Toolbar>

        {actionError && (
          <Alert variant="danger" isInline title="Action failed" style={{ marginBottom: 16 }}>
            {actionError}
          </Alert>
        )}

        {loadError ? (
          <EmptyState titleText="Cannot list desktops" headingLevel="h4">
            <EmptyStateBody>{String((loadError as Error)?.message ?? loadError)}</EmptyStateBody>
          </EmptyState>
        ) : !loaded ? (
          <Spinner aria-label="Loading desktops" />
        ) : rows.length === 0 ? (
          <EmptyState titleText="No desktops yet" headingLevel="h4">
            <EmptyStateBody>
              A desktop is a VdiInstance. Create one, then pair Moonlight with the PIN it shows.
            </EmptyStateBody>
          </EmptyState>
        ) : (
          <Table aria-label="Virtual desktops" variant="compact">
            <Thead>
              <Tr>
                <Th>Name</Th>
                {!namespace && <Th>Namespace</Th>}
                <Th>Status</Th>
                <Th>Class</Th>
                <Th>Session</Th>
                <Th>Moonlight address</Th>
                <Th>Owner</Th>
                <Th>Last active</Th>
                <Th screenReaderText="Actions" />
              </Tr>
            </Thead>
            <Tbody>
              {rows.map((vdi) => (
                <Tr key={`${vdi.metadata?.namespace}/${vdi.metadata?.name}`}>
                  <Td dataLabel="Name">
                    <ResourceLink
                      groupVersionKind={VdiInstanceGVK}
                      name={vdi.metadata?.name}
                      namespace={vdi.metadata?.namespace}
                    />
                  </Td>
                  {!namespace && <Td dataLabel="Namespace">{vdi.metadata?.namespace}</Td>}
                  <Td dataLabel="Status">
                    <DesktopStatus vdi={vdi} />
                  </Td>
                  <Td dataLabel="Class">{vdi.spec.className ?? '-'}</Td>
                  <Td dataLabel="Session">{vdi.status?.sessionState ?? '-'}</Td>
                  <Td dataLabel="Moonlight address">
                    {vdi.status?.address ? (
                      <ClipboardCopy hoverTip="Copy" clickTip="Copied" variant={ClipboardCopyVariant.inlineCompact} isReadOnly>
                        {vdi.status.address}
                      </ClipboardCopy>
                    ) : (
                      '-'
                    )}
                  </Td>
                  <Td dataLabel="Owner">{vdi.spec.owner ?? '-'}</Td>
                  <Td dataLabel="Last active">
                    {vdi.status?.lastActive ? <Timestamp timestamp={vdi.status.lastActive} /> : '-'}
                  </Td>
                  <Td isActionCell>
                    <ActionsColumn items={actions(vdi)} />
                  </Td>
                </Tr>
              ))}
            </Tbody>
          </Table>
        )}
      </PageSection>

      {pairing && <PairModal desktop={pairing} onClose={() => setPairing(null)} />}
      {creating && <CreateDesktopModal namespace={namespace} onClose={() => setCreating(false)} />}
      {deleting && (
        <Modal variant={ModalVariant.small} isOpen onClose={() => setDeleting(null)} aria-label="Delete desktop">
          <ModalHeader title={`Delete ${deleting.metadata?.name}?`} titleIconVariant="warning" />
          <ModalBody>The VM and its disk are deleted with it. This cannot be undone.</ModalBody>
          <ModalFooter>
            <Button variant="danger" onClick={confirmDelete}>
              Delete
            </Button>
            <Button variant="link" onClick={() => setDeleting(null)}>
              Cancel
            </Button>
          </ModalFooter>
        </Modal>
      )}
    </>
  );
};

export default DesktopsPage;
