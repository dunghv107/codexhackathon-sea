// Kiểu dữ liệu theo docs/API_CONTRACT.md. Tiền là string thập phân 2 chữ số.

export type Page<T> = { items: T[]; total: number; page: number; limit: number; total_pages: number }

export type Profile = { display_name: string; status: 'active' | 'suspended' }
export type Profiles = {
  buyer: (Profile & { buyer_id: string }) | null
  seller: (Profile & { seller_id: string }) | null
}

export type Category = { category_code: string; category_name_en: string | null }
export type CatalogShop = { shop_id: string; display_name: string; listing_count: number }
export type Shop = { shop_id: string; display_name: string; batch_id: string; assessed_orders: number }

export type ListingCard = {
  description: string | null
  image_url: string | null
  listing_id: string
  shop_id: string
  shop_name: string
  product_id: string
  product_name: string | null
  category_code: string | null
  category_name_en: string | null
  current_price: string | null
  currency: string
  stock_quantity: number | null
}

export type SavedAddress = {
  saved_address_id: string
  label: string
  address_raw: string
  region: string
  country_code: string
  is_default: boolean
  created_at: string
}

export type CheckoutResult = {
  checkout_id: string
  placed_at: string
  total_amount: string
  currency: string
  orders: { order_id: string; shop_id: string; shop_name: string; order_total: string }[]
}

export type OrderItem = {
  item_id: string
  listing_id: string
  product_name: string | null
  category_name_en: string | null
  quantity: number
  unit_price: string
  freight_amount: string
}
export type OrderAddress = {
  ward_code: string | null
  house_no: string | null
  parse_ok: boolean | null
  address_raw: string | null
  city: string | null
  region: string | null
  postal_prefix: string | null
  country_code: string | null
}

export type BuyerOrderSummary = {
  order_id: string
  checkout_id: string
  placed_at: string
  shop_id: string
  shop_name: string
  item_count: number
  order_total: string
  currency: string
  fulfillment_status: string
  display_status: string
}
export type BuyerOrderDetail = BuyerOrderSummary & {
  items: OrderItem[]
  address: OrderAddress
  payment: { method: string; status: string }
  events: { event_type: string; occurred_at: string }[]
}

export type SellerOrderSummary = {
  batch_id: string
  is_synthetic: boolean
  source_system: string
  order_id: string
  checkout_id: string
  placed_at: string
  payment_method: string
  item_count: number
  order_total: string
  currency: string
  review_status: string
  fulfillment_status: string
  risk_level: string | null
  score: number | null
}
export type Assessment = {
  rule_config: {
    status?: string
    score_cap?: number
    thresholds?: { medium_min: number; high_min: number }
    rules?: { rule_id: string; description: string; enabled: boolean; params?: Record<string, unknown> }[]
  }
  assessment_id: string
  assessed_at: string
  input_cutoff_at: string | null
  rule_version: string
  score: number
  risk_level: string
  feature_snapshot: unknown
  reasons: unknown
}
export type OrderCase = {
  case_id: string
  case_type: string
  opened_at: string
  available_at: string
  messages: { speaker_role: string; body: string; occurred_at: string; available_at: string }[]
  evidence: { media_type: string; content_ref: string; submitted_by: string; available_at: string }[]
  resolutions: { verification_status: string; responsible_party: string; rationale: string; decided_at: string; available_at: string }[]
}
export type SellerOrderDetail = SellerOrderSummary & {
  phone_masked: string
  demo_mode: boolean
  verification: OrderVerification | null
  cases: OrderCase[]
  items: OrderItem[]
  address: OrderAddress
  payment: { method: string; status: string }
  events: { event_type: string; actor_type: string; reason: string | null; occurred_at: string; available_at: string | null }[]
  latest_assessment: Assessment | null
  decisions: {
    decision_id: string
    assessment_id: string
    actor_type: string
    action: string
    reason: string
    created_at: string
  }[]
}

export type OrderVerification = {
  verification_id: string
  assessment_id: string
  status: 'pending' | 'confirmed' | 'declined' | 'unanswered'
  channel: 'demo'
  answered_at: string | null
}
export type VerificationView = { verification: OrderVerification | null; phone_masked: string }
export type RiskNetwork = {
  order_id: string
  input_cutoff_at: string
  nodes: { id: string; kind: 'checkout' | 'identity' | 'aggregate'; label: string; count: number }[]
  edges: { source: string; target: string }[]
  truncated: boolean
}

export type Dashboard = {
  review_status: Record<'pending' | 'hold' | 'released', number>
  fulfillment_status: Record<'unfulfilled' | 'packing' | 'shipped' | 'delivered' | 'canceled' | 'returned', number>
  risk_level: Record<'low' | 'medium' | 'high' | 'unassessed', number>
}
