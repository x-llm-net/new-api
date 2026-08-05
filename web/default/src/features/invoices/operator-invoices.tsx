/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  FileCheck2,
  FileText,
  RefreshCw,
  RotateCcw,
  XCircle,
} from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { SectionPageLayout } from '@/components/layout'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Textarea } from '@/components/ui/textarea'
import { TitledCard } from '@/components/ui/titled-card'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import {
  getOperatorInvoiceApplications,
  issueInvoiceApplication,
  redFlushInvoiceApplication,
  rejectInvoiceApplication,
} from './api'
import { InvoiceConfigCard } from './invoice-config-card'
import { InvoiceStatusBadge } from './invoice-status-badge'
import {
  formatInvoiceAmount,
  formatInvoiceTime,
  getInvoiceStatusLabel,
  invoiceStatuses,
} from './shared'
import type { InvoiceApplication, InvoiceStatus } from './types'

const PAGE_SIZE = 20
type ActionMode = 'issue' | 'reject' | 'red-flush'

export function OperatorInvoices() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const role = useAuthStore((state) => state.auth.user?.role ?? ROLE.GUEST)
  const [page, setPage] = useState(1)
  const [status, setStatus] = useState<InvoiceStatus | ''>('pending')
  const [selected, setSelected] = useState<InvoiceApplication | null>(null)
  const [actionMode, setActionMode] = useState<ActionMode>('issue')

  const applicationsQuery = useQuery({
    queryKey: ['invoice', 'operator', 'applications', page, status],
    queryFn: () => getOperatorInvoiceApplications(page, PAGE_SIZE, status),
    placeholderData: (previous) => previous,
  })

  const openAction = (application: InvoiceApplication, mode: ActionMode) => {
    setSelected(application)
    setActionMode(mode)
  }
  const closeAction = () => setSelected(null)
  const refresh = () =>
    queryClient.invalidateQueries({
      queryKey: ['invoice', 'operator', 'applications'],
    })

  const applications = applicationsQuery.data?.items ?? []
  const totalPages = Math.max(
    1,
    Math.ceil((applicationsQuery.data?.total ?? 0) / PAGE_SIZE)
  )

  return (
    <>
      <SectionPageLayout>
        <SectionPageLayout.Title>
          {t('Invoice management')}
        </SectionPageLayout.Title>
        <SectionPageLayout.Actions>
          <Button
            variant='outline'
            onClick={() => void refresh()}
            disabled={applicationsQuery.isFetching}
          >
            <RefreshCw
              className={applicationsQuery.isFetching ? 'animate-spin' : ''}
            />
            {t('Refresh')}
          </Button>
        </SectionPageLayout.Actions>
        <SectionPageLayout.Content>
          <div className='mx-auto flex w-full max-w-7xl flex-col gap-4'>
            {role === ROLE.SUPER_ADMIN && <InvoiceConfigCard />}

            <TitledCard
              title={t('Invoice applications')}
              description={t(
                'Review pending applications and manage issued invoices.'
              )}
              icon={<FileText />}
              iconTone='info'
              disableHoverEffect
              contentClassName='p-0'
              action={
                <NativeSelect
                  aria-label={t('Filter by status')}
                  className='w-full sm:w-44'
                  value={status}
                  onChange={(event) => {
                    setStatus(event.target.value as InvoiceStatus | '')
                    setPage(1)
                  }}
                >
                  <NativeSelectOption value=''>
                    {t('All statuses')}
                  </NativeSelectOption>
                  {invoiceStatuses.map((value) => (
                    <NativeSelectOption key={value} value={value}>
                      {getInvoiceStatusLabel(value, t)}
                    </NativeSelectOption>
                  ))}
                </NativeSelect>
              }
            >
              {applicationsQuery.isLoading && (
                <div className='space-y-3 p-4'>
                  {['first', 'second', 'third', 'fourth', 'fifth'].map(
                    (key) => (
                      <Skeleton key={key} className='h-14 w-full' />
                    )
                  )}
                </div>
              )}
              {!applicationsQuery.isLoading && applications.length === 0 && (
                <div className='text-muted-foreground px-4 py-12 text-center text-sm'>
                  {t('No invoice applications found')}
                </div>
              )}
              {!applicationsQuery.isLoading && applications.length > 0 && (
                <>
                  <div className='hidden overflow-x-auto lg:block'>
                    <Table>
                      <TableHeader>
                        <TableRow>
                          <TableHead>{t('Application No.')}</TableHead>
                          <TableHead>{t('User')}</TableHead>
                          <TableHead>{t('Amount')}</TableHead>
                          <TableHead>{t('Invoice information')}</TableHead>
                          <TableHead>{t('Status')}</TableHead>
                          <TableHead>{t('Submitted at')}</TableHead>
                          <TableHead className='text-right'>
                            {t('Actions')}
                          </TableHead>
                        </TableRow>
                      </TableHeader>
                      <TableBody>
                        {applications.map((application) => (
                          <TableRow key={application.id}>
                            <TableCell className='font-mono text-xs'>
                              {application.request_no}
                            </TableCell>
                            <TableCell>
                              <div className='font-medium'>
                                {application.username || '-'}
                              </div>
                              <div className='text-muted-foreground text-xs'>
                                ID {application.user_id}
                              </div>
                            </TableCell>
                            <TableCell className='font-medium tabular-nums'>
                              {formatInvoiceAmount(application.amount_cents)}
                            </TableCell>
                            <TableCell>
                              <InvoiceDetails application={application} />
                            </TableCell>
                            <TableCell>
                              <InvoiceStatusBadge status={application.status} />
                            </TableCell>
                            <TableCell className='text-muted-foreground whitespace-nowrap'>
                              {formatInvoiceTime(application.created_at)}
                            </TableCell>
                            <TableCell>
                              <OperatorActions
                                application={application}
                                onAction={openAction}
                              />
                            </TableCell>
                          </TableRow>
                        ))}
                      </TableBody>
                    </Table>
                  </div>

                  <div className='divide-y lg:hidden'>
                    {applications.map((application) => (
                      <div key={application.id} className='space-y-3 p-4'>
                        <div className='flex items-start justify-between gap-3'>
                          <div className='min-w-0'>
                            <div className='truncate font-medium'>
                              {application.username || '-'}
                              <span className='text-muted-foreground ml-1.5 text-xs font-normal'>
                                ID {application.user_id}
                              </span>
                            </div>
                            <div className='text-muted-foreground mt-0.5 font-mono text-xs'>
                              {application.request_no}
                            </div>
                          </div>
                          <InvoiceStatusBadge status={application.status} />
                        </div>
                        <InvoiceDetails application={application} />
                        <div className='flex flex-wrap items-end justify-between gap-3'>
                          <div>
                            <div className='font-mono text-lg font-semibold tabular-nums'>
                              {formatInvoiceAmount(application.amount_cents)}
                            </div>
                            <div className='text-muted-foreground text-xs'>
                              {formatInvoiceTime(application.created_at)}
                            </div>
                          </div>
                          <OperatorActions
                            application={application}
                            onAction={openAction}
                          />
                        </div>
                      </div>
                    ))}
                  </div>
                </>
              )}

              {(applicationsQuery.data?.total ?? 0) > PAGE_SIZE && (
                <div className='flex items-center justify-between border-t px-4 py-3'>
                  <span className='text-muted-foreground text-sm'>
                    {t('Page {{page}} of {{total}}', {
                      page,
                      total: totalPages,
                    })}
                  </span>
                  <div className='flex gap-2'>
                    <Button
                      variant='outline'
                      size='sm'
                      disabled={page <= 1}
                      onClick={() => setPage((value) => value - 1)}
                    >
                      {t('Previous')}
                    </Button>
                    <Button
                      variant='outline'
                      size='sm'
                      disabled={page >= totalPages}
                      onClick={() => setPage((value) => value + 1)}
                    >
                      {t('Next')}
                    </Button>
                  </div>
                </div>
              )}
            </TitledCard>
          </div>
        </SectionPageLayout.Content>
      </SectionPageLayout>

      <InvoiceActionDialog
        application={selected}
        mode={actionMode}
        onClose={closeAction}
        onCompleted={async () => {
          closeAction()
          await refresh()
        }}
      />
    </>
  )
}

function InvoiceDetails({ application }: { application: InvoiceApplication }) {
  const { t } = useTranslation()
  return (
    <div className='max-w-72 space-y-0.5 text-sm'>
      <div className='truncate font-medium'>{application.title_name}</div>
      {application.tax_number && (
        <div className='text-muted-foreground truncate text-xs'>
          {t('Tax number')}: {application.tax_number}
        </div>
      )}
      <div className='text-muted-foreground truncate text-xs'>
        {application.email}
      </div>
      {application.reject_reason && (
        <div className='text-destructive truncate text-xs'>
          {application.reject_reason}
        </div>
      )}
    </div>
  )
}

function OperatorActions({
  application,
  onAction,
}: {
  application: InvoiceApplication
  onAction: (application: InvoiceApplication, mode: ActionMode) => void
}) {
  const { t } = useTranslation()
  if (application.status === 'pending') {
    return (
      <div className='flex justify-end gap-2'>
        <Button
          variant='outline'
          size='sm'
          onClick={() => onAction(application, 'reject')}
        >
          <XCircle />
          {t('Reject')}
        </Button>
        <Button size='sm' onClick={() => onAction(application, 'issue')}>
          <FileCheck2 />
          {t('Issue')}
        </Button>
      </div>
    )
  }
  if (application.status === 'issued') {
    return (
      <div className='flex justify-end'>
        <Button
          variant='outline'
          size='sm'
          onClick={() => onAction(application, 'red-flush')}
        >
          <RotateCcw />
          {t('Red flush')}
        </Button>
      </div>
    )
  }
  return null
}

function InvoiceActionDialog({
  application,
  mode,
  onClose,
  onCompleted,
}: {
  application: InvoiceApplication | null
  mode: ActionMode
  onClose: () => void
  onCompleted: () => Promise<void>
}) {
  const { t } = useTranslation()
  const [invoiceNumber, setInvoiceNumber] = useState('')
  const [file, setFile] = useState<File | null>(null)
  const [reason, setReason] = useState('')

  const issueMutation = useMutation({
    mutationFn: () => {
      if (!application || !file) {
        throw new Error('Invoice application or file is missing')
      }
      return issueInvoiceApplication(application.id, invoiceNumber.trim(), file)
    },
    onSuccess: async () => {
      toast.success(t('Invoice marked as issued'))
      setInvoiceNumber('')
      setFile(null)
      await onCompleted()
    },
  })
  const rejectMutation = useMutation({
    mutationFn: () => {
      if (!application) throw new Error('Invoice application is missing')
      return rejectInvoiceApplication(application.id, reason.trim())
    },
    onSuccess: async () => {
      toast.success(t('Invoice application rejected'))
      setReason('')
      await onCompleted()
    },
  })
  const redFlushMutation = useMutation({
    mutationFn: () => {
      if (!application) throw new Error('Invoice application is missing')
      return redFlushInvoiceApplication(application.id)
    },
    onSuccess: async () => {
      toast.success(t('Invoice marked as red-flushed'))
      await onCompleted()
    },
  })

  const pending =
    issueMutation.isPending ||
    rejectMutation.isPending ||
    redFlushMutation.isPending

  const submit = () => {
    if (mode === 'issue') {
      if (!invoiceNumber.trim() || !file) {
        toast.error(t('Enter the invoice number and select a PDF file'))
        return
      }
      if (
        file.size > 10 * 1024 * 1024 ||
        (!file.name.toLowerCase().endsWith('.pdf') &&
          file.type !== 'application/pdf')
      ) {
        toast.error(t('Select a PDF file no larger than 10 MB'))
        return
      }
      issueMutation.mutate()
      return
    }
    if (mode === 'reject') {
      if (!reason.trim()) {
        toast.error(t('Enter a rejection reason'))
        return
      }
      rejectMutation.mutate()
      return
    }
    redFlushMutation.mutate()
  }

  let title = t('Confirm red flush')
  if (mode === 'issue') title = t('Issue invoice')
  if (mode === 'reject') title = t('Reject invoice application')

  return (
    <Dialog
      open={application !== null}
      onOpenChange={(open) => !open && onClose()}
    >
      <DialogContent className='sm:max-w-lg'>
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>
            {application
              ? t('{{number}} · {{amount}}', {
                  number: application.request_no,
                  amount: formatInvoiceAmount(application.amount_cents),
                })
              : ''}
          </DialogDescription>
        </DialogHeader>

        {mode === 'issue' && (
          <div className='grid gap-4 py-1'>
            <div className='grid gap-2'>
              <Label htmlFor='invoice-number'>{t('Invoice number')}</Label>
              <Input
                id='invoice-number'
                maxLength={128}
                value={invoiceNumber}
                onChange={(event) => setInvoiceNumber(event.target.value)}
              />
            </div>
            <div className='grid gap-2'>
              <Label htmlFor='invoice-file'>{t('Invoice PDF')}</Label>
              <Input
                id='invoice-file'
                type='file'
                accept='application/pdf,.pdf'
                onChange={(event) => setFile(event.target.files?.[0] ?? null)}
              />
              <p className='text-muted-foreground text-xs'>
                {t('PDF only, up to 10 MB')}
              </p>
            </div>
          </div>
        )}

        {mode === 'reject' && (
          <div className='grid gap-2 py-1'>
            <Label htmlFor='invoice-reject-reason'>
              {t('Rejection reason')}
            </Label>
            <Textarea
              id='invoice-reject-reason'
              maxLength={500}
              rows={4}
              value={reason}
              onChange={(event) => setReason(event.target.value)}
            />
          </div>
        )}

        {mode === 'red-flush' && (
          <p className='text-muted-foreground py-1 text-sm'>
            {t(
              'Confirm only after the invoice has been red-flushed. The amount will become available again.'
            )}
          </p>
        )}

        <DialogFooter>
          <DialogClose render={<Button variant='outline' disabled={pending} />}>
            {t('Cancel')}
          </DialogClose>
          <Button
            variant={mode === 'reject' ? 'destructive' : 'default'}
            disabled={pending}
            onClick={submit}
          >
            {title}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
