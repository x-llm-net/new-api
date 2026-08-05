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
import { Download, FileText, Plus, X } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { SectionPageLayout } from '@/components/layout'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
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
import { TitledCard } from '@/components/ui/titled-card'
import { useAuthStore } from '@/stores/auth-store'

import {
  cancelInvoiceApplication,
  createInvoiceApplication,
  downloadInvoice,
  getInvoiceSummary,
  getMyInvoiceApplications,
} from './api'
import { InvoiceStatusBadge } from './invoice-status-badge'
import { formatInvoiceAmount, formatInvoiceTime } from './shared'
import type {
  InvoiceSummary as InvoiceSummaryData,
  InvoiceTitleType,
} from './types'

const PAGE_SIZE = 20

export function UserInvoices() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const user = useAuthStore((state) => state.auth.user)
  const [page, setPage] = useState(1)
  const [dialogOpen, setDialogOpen] = useState(false)
  const [amount, setAmount] = useState('')
  const [titleType, setTitleType] = useState<InvoiceTitleType>('company')
  const [titleName, setTitleName] = useState('')
  const [taxNumber, setTaxNumber] = useState('')
  const [email, setEmail] = useState(user?.email || '')

  const summaryQuery = useQuery({
    queryKey: ['invoice', 'summary'],
    queryFn: getInvoiceSummary,
  })
  const applicationsQuery = useQuery({
    queryKey: ['invoice', 'applications', page],
    queryFn: () => getMyInvoiceApplications(page, PAGE_SIZE),
    placeholderData: (previous) => previous,
  })

  const refresh = async () => {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: ['invoice', 'summary'] }),
      queryClient.invalidateQueries({ queryKey: ['invoice', 'applications'] }),
    ])
  }

  const createMutation = useMutation({
    mutationFn: createInvoiceApplication,
    onSuccess: async () => {
      toast.success(t('Invoice application submitted'))
      setDialogOpen(false)
      setAmount('')
      setTitleName('')
      setTaxNumber('')
      await refresh()
    },
  })
  const cancelMutation = useMutation({
    mutationFn: cancelInvoiceApplication,
    onSuccess: async () => {
      toast.success(t('Invoice application cancelled'))
      await refresh()
    },
  })

  const summary = summaryQuery.data
  const applications = applicationsQuery.data?.items ?? []
  const totalPages = Math.max(
    1,
    Math.ceil((applicationsQuery.data?.total ?? 0) / PAGE_SIZE)
  )

  const openApplicationDialog = () => {
    if (summary) {
      setAmount((summary.available_amount_cents / 100).toFixed(2))
    }
    setDialogOpen(true)
  }

  const submitApplication = () => {
    const numericAmount = Number(amount)
    const amountCents = Math.round(numericAmount * 100)
    if (!Number.isFinite(numericAmount) || amountCents <= 0) {
      toast.error(t('Enter a valid invoice amount'))
      return
    }
    if (!summary || amountCents > summary.available_amount_cents) {
      toast.error(t('Invoice amount exceeds the available amount'))
      return
    }
    if (amountCents < summary.min_amount_cents) {
      toast.error(
        t('Minimum invoice amount is {{amount}}', {
          amount: formatInvoiceAmount(summary.min_amount_cents),
        })
      )
      return
    }
    if (!titleName.trim() || !email.trim()) {
      toast.error(t('Complete the required invoice information'))
      return
    }
    if (titleType === 'company' && !taxNumber.trim()) {
      toast.error(t('Tax number is required for a company invoice'))
      return
    }

    createMutation.mutate({
      amount_cents: amountCents,
      title_type: titleType,
      title_name: titleName.trim(),
      tax_number: titleType === 'company' ? taxNumber.trim() : '',
      email: email.trim(),
      idempotency_key:
        typeof crypto.randomUUID === 'function'
          ? crypto.randomUUID()
          : `${Date.now()}-${Math.random().toString(36).slice(2)}`,
    })
  }

  return (
    <>
      <SectionPageLayout>
        <SectionPageLayout.Title>{t('Invoices')}</SectionPageLayout.Title>
        <SectionPageLayout.Actions>
          <Button
            onClick={openApplicationDialog}
            disabled={
              !summary?.enabled ||
              summary.available_amount_cents < summary.min_amount_cents
            }
          >
            <Plus />
            {t('Apply for invoice')}
          </Button>
        </SectionPageLayout.Actions>
        <SectionPageLayout.Content>
          <div className='mx-auto flex w-full max-w-7xl flex-col gap-4'>
            {!summaryQuery.isLoading && summary && !summary.enabled && (
              <Alert>
                <FileText />
                <AlertTitle>{t('Invoice applications are paused')}</AlertTitle>
                <AlertDescription>
                  {t(
                    'Existing applications and issued invoices remain available.'
                  )}
                </AlertDescription>
              </Alert>
            )}

            <InvoiceSummary
              summary={summary}
              loading={summaryQuery.isLoading}
            />

            <TitledCard
              title={t('Invoice history')}
              description={t(
                'Review application status and download issued invoices.'
              )}
              icon={<FileText />}
              iconTone='info'
              disableHoverEffect
              contentClassName='p-0'
            >
              {applicationsQuery.isLoading && (
                <div className='space-y-3 p-4'>
                  {['first', 'second', 'third', 'fourth'].map((key) => (
                    <Skeleton key={key} className='h-12 w-full' />
                  ))}
                </div>
              )}
              {!applicationsQuery.isLoading && applications.length === 0 && (
                <div className='text-muted-foreground px-4 py-12 text-center text-sm'>
                  {t('No invoice applications yet')}
                </div>
              )}
              {!applicationsQuery.isLoading && applications.length > 0 && (
                <>
                  <div className='hidden overflow-x-auto md:block'>
                    <Table>
                      <TableHeader>
                        <TableRow>
                          <TableHead>{t('Application No.')}</TableHead>
                          <TableHead>{t('Amount')}</TableHead>
                          <TableHead>{t('Invoice title')}</TableHead>
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
                            <TableCell className='font-medium tabular-nums'>
                              {formatInvoiceAmount(application.amount_cents)}
                            </TableCell>
                            <TableCell>
                              <div className='max-w-56 truncate font-medium'>
                                {application.title_name}
                              </div>
                              <div className='text-muted-foreground max-w-56 truncate text-xs'>
                                {application.invoice_content}
                              </div>
                            </TableCell>
                            <TableCell>
                              <InvoiceStatusBadge status={application.status} />
                            </TableCell>
                            <TableCell className='text-muted-foreground whitespace-nowrap'>
                              {formatInvoiceTime(application.created_at)}
                            </TableCell>
                            <TableCell>
                              <InvoiceActions
                                application={application}
                                cancelling={cancelMutation.isPending}
                                onCancel={() =>
                                  cancelMutation.mutate(application.id)
                                }
                              />
                            </TableCell>
                          </TableRow>
                        ))}
                      </TableBody>
                    </Table>
                  </div>

                  <div className='divide-y md:hidden'>
                    {applications.map((application) => (
                      <div key={application.id} className='space-y-3 p-4'>
                        <div className='flex items-start justify-between gap-3'>
                          <div className='min-w-0'>
                            <div className='truncate font-medium'>
                              {application.title_name}
                            </div>
                            <div className='text-muted-foreground mt-0.5 font-mono text-xs'>
                              {application.request_no}
                            </div>
                          </div>
                          <InvoiceStatusBadge status={application.status} />
                        </div>
                        <div className='flex items-end justify-between gap-3'>
                          <div>
                            <div className='font-mono text-lg font-semibold tabular-nums'>
                              {formatInvoiceAmount(application.amount_cents)}
                            </div>
                            <div className='text-muted-foreground text-xs'>
                              {formatInvoiceTime(application.created_at)}
                            </div>
                          </div>
                          <InvoiceActions
                            application={application}
                            cancelling={cancelMutation.isPending}
                            onCancel={() =>
                              cancelMutation.mutate(application.id)
                            }
                          />
                        </div>
                        {application.reject_reason && (
                          <div className='bg-destructive/5 text-destructive rounded-md px-3 py-2 text-xs'>
                            {application.reject_reason}
                          </div>
                        )}
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

      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent className='sm:max-w-lg'>
          <DialogHeader>
            <DialogTitle>{t('Apply for invoice')}</DialogTitle>
            <DialogDescription>
              {t(
                'The requested amount is reserved immediately after submission.'
              )}
            </DialogDescription>
          </DialogHeader>

          <div className='grid gap-4 py-1'>
            <div className='grid gap-2'>
              <Label htmlFor='invoice-amount'>{t('Invoice amount')}</Label>
              <Input
                id='invoice-amount'
                type='number'
                min={(summary?.min_amount_cents ?? 0) / 100}
                max={(summary?.available_amount_cents ?? 0) / 100}
                step='0.01'
                value={amount}
                onChange={(event) => setAmount(event.target.value)}
              />
              <p className='text-muted-foreground text-xs'>
                {t('Available: {{amount}}', {
                  amount: formatInvoiceAmount(
                    summary?.available_amount_cents ?? 0
                  ),
                })}
              </p>
            </div>

            <div className='grid gap-2'>
              <Label htmlFor='invoice-title-type'>{t('Title type')}</Label>
              <NativeSelect
                id='invoice-title-type'
                className='w-full'
                value={titleType}
                onChange={(event) =>
                  setTitleType(event.target.value as InvoiceTitleType)
                }
              >
                <NativeSelectOption value='company'>
                  {t('Company')}
                </NativeSelectOption>
                <NativeSelectOption value='personal'>
                  {t('Personal')}
                </NativeSelectOption>
              </NativeSelect>
            </div>

            <div className='grid gap-2'>
              <Label htmlFor='invoice-title'>{t('Invoice title')}</Label>
              <Input
                id='invoice-title'
                value={titleName}
                maxLength={191}
                onChange={(event) => setTitleName(event.target.value)}
              />
            </div>

            {titleType === 'company' && (
              <div className='grid gap-2'>
                <Label htmlFor='invoice-tax-number'>{t('Tax number')}</Label>
                <Input
                  id='invoice-tax-number'
                  value={taxNumber}
                  maxLength={64}
                  onChange={(event) => setTaxNumber(event.target.value)}
                />
              </div>
            )}

            <div className='grid gap-2'>
              <Label htmlFor='invoice-email'>{t('Email')}</Label>
              <Input
                id='invoice-email'
                type='email'
                value={email}
                maxLength={191}
                onChange={(event) => setEmail(event.target.value)}
              />
            </div>
          </div>

          <DialogFooter>
            <DialogClose
              render={
                <Button variant='outline' disabled={createMutation.isPending} />
              }
            >
              {t('Cancel')}
            </DialogClose>
            <Button
              onClick={submitApplication}
              disabled={createMutation.isPending}
            >
              {t('Submit')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}

function InvoiceSummary({
  summary,
  loading,
}: {
  summary?: InvoiceSummaryData
  loading: boolean
}) {
  const { t } = useTranslation()
  const items = [
    {
      label: t('Total eligible payments'),
      amount: summary?.eligible_amount_cents ?? 0,
    },
    {
      label: t('Pending amount'),
      amount: summary?.pending_amount_cents ?? 0,
    },
    {
      label: t('Issued amount'),
      amount: summary?.issued_amount_cents ?? 0,
    },
    {
      label: t('Available invoice amount'),
      amount: summary?.available_amount_cents ?? 0,
    },
  ]

  return (
    <div className='grid grid-cols-2 divide-x divide-y rounded-lg border lg:grid-cols-4 lg:divide-y-0'>
      {items.map((item) => (
        <div key={item.label} className='min-w-0 px-4 py-4 sm:px-5'>
          <div className='text-muted-foreground truncate text-xs font-medium'>
            {item.label}
          </div>
          {loading ? (
            <Skeleton className='mt-2 h-7 w-28' />
          ) : (
            <div className='mt-1.5 font-mono text-xl font-semibold tracking-normal break-all tabular-nums sm:text-2xl'>
              {formatInvoiceAmount(item.amount)}
            </div>
          )}
        </div>
      ))}
    </div>
  )
}

function InvoiceActions({
  application,
  cancelling,
  onCancel,
}: {
  application: {
    id: number
    request_no: string
    status: string
    has_file: boolean
  }
  cancelling: boolean
  onCancel: () => void
}) {
  const { t } = useTranslation()
  return (
    <div className='flex justify-end gap-2'>
      {application.status === 'issued' && application.has_file && (
        <Button
          variant='outline'
          size='sm'
          onClick={() =>
            void downloadInvoice(application.id, application.request_no)
          }
        >
          <Download />
          {t('Download')}
        </Button>
      )}
      {application.status === 'pending' && (
        <Button
          variant='ghost'
          size='sm'
          disabled={cancelling}
          onClick={onCancel}
        >
          <X />
          {t('Cancel')}
        </Button>
      )}
    </div>
  )
}
