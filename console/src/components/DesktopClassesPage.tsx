import type { FC } from 'react';
import { EmptyState, EmptyStateBody, PageSection, Spinner } from '@patternfly/react-core';
import { Table, Tbody, Td, Th, Thead, Tr } from '@patternfly/react-table';
import { ListPageCreate, ListPageHeader, ResourceLink } from '@openshift-console/dynamic-plugin-sdk';
import { VdiClassGVK } from '../constants';
import { placementText, sizeText } from '../format';
import { useVdiClasses } from '../hooks/useVdiClasses';

const DesktopClassesPage: FC = () => {
  const [classes, loaded, loadError] = useVdiClasses();

  return (
    <>
      <ListPageHeader title="Desktop classes" helpText="A desktop takes from its class what it does not set itself.">
        <ListPageCreate groupVersionKind={VdiClassGVK} createAccessReview={{ groupVersionKind: VdiClassGVK }}>
          Create class
        </ListPageCreate>
      </ListPageHeader>
      <PageSection>
        {loadError ? (
          <EmptyState titleText="Cannot list desktop classes" headingLevel="h4">
            <EmptyStateBody>{String((loadError as Error)?.message ?? loadError)}</EmptyStateBody>
          </EmptyState>
        ) : !loaded ? (
          <Spinner aria-label="Loading desktop classes" />
        ) : classes.length === 0 ? (
          <EmptyState titleText="No desktop classes yet" headingLevel="h4">
            <EmptyStateBody>Create one, then pick it when you create a desktop.</EmptyStateBody>
          </EmptyState>
        ) : (
          <Table aria-label="Desktop classes" variant="compact">
            <Thead>
              <Tr>
                <Th>Name</Th>
                <Th>Image</Th>
                <Th>Size</Th>
                <Th>Placement</Th>
              </Tr>
            </Thead>
            <Tbody>
              {classes.map((c) => (
                <Tr key={c.metadata?.name}>
                  <Td dataLabel="Name">
                    <ResourceLink groupVersionKind={VdiClassGVK} name={c.metadata?.name} />
                  </Td>
                  <Td dataLabel="Image">{c.spec.image}</Td>
                  <Td dataLabel="Size">{sizeText(c.spec.resources) ?? '-'}</Td>
                  <Td dataLabel="Placement">{placementText(c.spec.placement)}</Td>
                </Tr>
              ))}
            </Tbody>
          </Table>
        )}
      </PageSection>
    </>
  );
};

export default DesktopClassesPage;
