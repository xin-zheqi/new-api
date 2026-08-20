export type InvoiceSubscription = {
  id: number
  plan_title: string
  amount_total: number
  start_time: number
  end_time: number
  created_at: number
}

export type InvoiceItem = {
  id: number
  user_subscription_id: number
  plan_title: string
  amount_total: number
}

export type InvoiceApplication = {
  id: number
  user_id: number
  invoice_title: string
  total_amount: number
  status: 'pending' | 'completed'
  pdf_name?: string
  created_at: number
  completed_at: number
  items: InvoiceItem[]
  user?: { username: string; display_name?: string; email?: string }
}

export type InvoiceCenterData = {
  application_day: number
  lookback_days: number
  monthly_limit: number
  subscriptions: InvoiceSubscription[]
  applications: InvoiceApplication[]
}
