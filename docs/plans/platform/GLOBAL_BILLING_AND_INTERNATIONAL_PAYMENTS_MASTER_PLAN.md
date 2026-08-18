# CodeLocal Global Billing & International Payments Master Plan

Status: **Approved for implementation**  
Date: **2026-08-19**  
Target release path: **stage on current branch, integrate into `dev` through a clean reviewed change**  
Owner: **CodeLocal**

This document is the source of truth for turning CodeLocal from a globally usable product into a product that can reliably charge international customers while the founder/business operates from Vietnam.

---

# 1. Product decision

## Primary provider — Lemon Squeezy

Use **Lemon Squeezy as the first production Merchant of Record (MoR)** for CodeLocal.

Reasoning:

- Vietnam is listed as a supported country for bank payouts.
- Software and SaaS are supported product types.
- Lemon Squeezy acts as Merchant of Record for the customer transaction, including payment collection, sales tax/VAT handling, refunds/chargebacks and PCI obligations on the transaction side.
- It supports recurring subscriptions, hosted checkout, API, webhooks and a hosted Customer Portal.
- It lets CodeLocal launch without requiring a US company solely for payment processing.

Do **not** couple CodeLocal's internal billing model directly to Lemon Squeezy object shapes. Lemon Squeezy is the first adapter, not the domain model.

## Secondary / migration providers

Keep provider abstraction compatible with:

1. **Paddle** — future MoR alternative, especially for larger B2B/SaaS requirements.
2. **Stripe** — future direct processor after CodeLocal has a supported legal entity/banking setup. As of 2026-08-19, Stripe's global payments availability page does not list Vietnam as a directly supported payments country.

Do not fake an overseas address, company, director, bank account or tax identity to access Stripe.

---

# 2. Important CodeLocal compliance positioning

CodeLocal must be described accurately during merchant review.

Recommended product positioning:

> CodeLocal is developer productivity software that connects an AI coding assistant to projects and local tools explicitly authorized by the user. It provides persistent project intelligence and controlled local execution. The user selects/authorizes the workspace and retains control over permissions and actions.

Avoid ambiguous descriptions such as:

- "remote computer control service";
- "control any computer invisibly";
- "monitor employee computers";
- "spy on devices";
- "bypass user permission".

Lemon Squeezy allows Software & SaaS but lists spyware/parental-control apps among prohibited products. CodeLocal's approval and permission model must therefore be visible in the website, product description, privacy policy and onboarding material.

Before activating the merchant account, confirm the live website clearly contains:

- product explanation;
- pricing;
- terms of service;
- privacy policy;
- refund/cancellation policy;
- support/contact method;
- clear explanation that local access requires explicit user authorization.

If merchant review has any doubt about CodeLocal's computer-control capabilities, contact Lemon Squeezy support with a precise product description before taking production payments.

---

# 3. Founder setup from Vietnam — operational checklist

## Phase A — Prepare the business-facing website

- [ ] `codelocal.cloud` has an English landing page.
- [ ] Product has a clear SaaS description.
- [ ] Pricing page exists and matches the plans we intend to sell.
- [ ] Terms of Service published.
- [ ] Privacy Policy published.
- [ ] Refund / cancellation policy published.
- [ ] Support email/contact path published.
- [ ] Screenshots/demo make it clear CodeLocal is developer software, not surveillance software.
- [ ] Explicit permission/authorization model is explained.

## Phase B — Create Lemon Squeezy account/store

- [ ] Create the merchant account using real Vietnam identity/business information.
- [ ] Create a store for CodeLocal.
- [ ] Set store country/currency deliberately.
- [ ] Enable 2FA.
- [ ] Complete required identity/business verification (KYC/KYB).
- [ ] Complete the non-US tax form requested by Lemon Squeezy for the actual seller type.
- [ ] Configure a supported Vietnam payout destination.
- [ ] Prefer verified bank payout unless there is a concrete reason to use PayPal.
- [ ] Activate the live store after product/site information is complete.

Official documentation says Lemon Squeezy reviews stores because it is the Merchant of Record and performs KYC/KYB checks. Store approval is not assumed until activation succeeds.

## Phase C — Payout expectations

Current official payout behavior to design cash-flow expectations around:

- store payouts are created twice monthly, on the 1st and 15th;
- net sales are held for 13 days before becoming available for payout;
- bank payouts may take approximately 1–5 days to arrive;
- bank payouts outside the US can have an additional payout fee;
- currency conversion may apply when settling into the local bank currency.

Do not design CodeLocal's own cash-flow assumptions as if card revenue were instant cash in the Vietnam bank account.

---

# 4. Initial CodeLocal commercial model

Initial recommendation:

```text
Free
  -> evaluation / limited usage

Pro
  -> individual paid subscription

Team
  -> multi-user / seat-based subscription

Business / Enterprise
  -> later, sales-assisted if needed
```

The exact price is a product decision and may change without changing the billing architecture.

For the first implementation, prefer simple recurring subscription tiers over usage-based billing. Metered billing can be added later after usage authority and cost accounting are stable.

Important: CodeLocal's existing telemetry path is explicitly non-authoritative. **Never use droppable telemetry as the billing source of truth.**

---

# 5. Billing domain architecture

```text
                       codelocal.cloud
                             |
                             v
                         Pricing UI
                             |
                             v
                    Billing / Checkout API
                             |
                  +----------+-----------+
                  |                      |
                  v                      v
          Lemon Squeezy adapter     future adapters
                                    Paddle / Stripe
                  |
                  v
            Hosted Checkout
                  |
                  v
        customer payment succeeds
                  |
                  v
          signed webhook event
                  |
                  v
           Billing Event Inbox
                  |
                  v
          normalized subscription
                  |
                  v
            Entitlement Engine
                  |
          +-------+--------+
          |                |
          v                v
       access           plan/quota
```

The browser must never become the authority for paid access.

---

# 6. Internal provider-neutral data model

Minimum domain entities:

## BillingCustomer

```text
id
userId / organizationId
provider
providerCustomerId
email
createdAt
updatedAt
```

## Subscription

```text
id
billingCustomerId
provider
providerSubscriptionId
providerProductId
providerVariantOrPriceId
planCode
status
currentPeriodStart
currentPeriodEnd
cancelAtPeriodEnd
createdAt
updatedAt
```

## Entitlement

```text
subjectType: user | organization
subjectId
featureCode
planCode
state
validFrom
validUntil
source: billing | admin | promotion
sourceReference
updatedAt
```

## BillingEvent

```text
provider
providerEventId / deterministic event fingerprint
eventType
receivedAt
processedAt
status
attemptCount
payloadHash
lastError
```

Raw provider secrets never enter user-visible logs or Project Brain memory.

---

# 7. Checkout flow

Recommended V1 flow:

```text
Authenticated user
   |
   v
Choose Pro / Team
   |
   v
POST /api/billing/checkout
   |
   | server resolves current user/org
   | server maps internal planCode -> provider variant
   | server adds trusted custom metadata
   v
Lemon Squeezy hosted checkout
   |
   v
Payment success
   |
   +------> browser success page (UX only)
   |
   v
Webhook
   |
   v
server verifies signature
   |
   v
normalize + idempotently persist event
   |
   v
subscription state update
   |
   v
entitlement update
```

**Non-negotiable:** redirecting to a success page must not itself unlock Pro. Only verified server-side billing state may grant paid entitlements.

Pass a stable internal user/org reference through supported checkout custom data so the webhook can safely bind the purchase back to CodeLocal.

---

# 8. Webhook contract

Initial Lemon Squeezy events to support:

```text
subscription_created
subscription_updated
subscription_cancelled
subscription_resumed
subscription_expired
subscription_paused
subscription_unpaused
subscription_payment_success
subscription_payment_failed
subscription_payment_recovered
order_refunded
```

Minimum security and reliability rules:

- [ ] Verify webhook signature before parsing it as trusted state.
- [ ] Reject invalid signatures.
- [ ] Enforce body-size limit.
- [ ] Process idempotently.
- [ ] Persist an event/fingerprint before applying mutable state.
- [ ] Duplicate delivery must not duplicate entitlements or seats.
- [ ] Event ordering must not blindly regress newer subscription state.
- [ ] Return success only after the event is durably accepted.
- [ ] Move heavy downstream work off the request path where possible.
- [ ] Keep a replay/reconciliation path.
- [ ] Store structured errors without secrets/card data.

Lemon Squeezy retries failed webhook deliveries, so duplicate-safe processing is mandatory.

---

# 9. Subscription state -> CodeLocal entitlement mapping

Provider state should be normalized before affecting authorization.

Example policy:

```text
active / trialing (if enabled)
  -> entitlement ACTIVE

cancelled but still inside paid period
  -> entitlement ACTIVE until currentPeriodEnd

past_due / payment_failed
  -> grace policy, not immediate destructive deletion

expired
  -> entitlement INACTIVE

refunded
  -> apply explicit refund/revocation policy
```

Never delete project knowledge because a subscription ends. Paid capability and durable user-owned data lifecycle are separate concerns.

Downgrades should preserve user data wherever practical and disable paid-only operations according to an explicit product policy.

---

# 10. Customer Billing UI

Add a restrained billing section in CodeLocal dashboard/account:

```text
Plan
  Pro

Status
  Active

Renews
  <date>

[ Manage subscription ]
[ Update payment method ]
```

V1 should reuse Lemon Squeezy's hosted Customer Portal/signed portal URL rather than building an entire card-management system.

Customer Portal can handle subscription management, billing information, payment methods, receipts/invoices and cancellation/plan changes depending on configured options.

CodeLocal should still show its own normalized plan/status because authorization comes from CodeLocal's backend state, not from iframe/browser assumptions.

---

# 11. Pricing and fee model

Current Lemon Squeezy public base pricing is **5% + $0.50 per transaction**, with documented additional fees in some cases, including international transactions, PayPal and subscription payments.

Therefore:

- never hard-code gross price = net revenue;
- store gross, tax, provider fees and net payout data separately when reconciliation data is available;
- pricing experiments should account for fixed $0.50 cost, especially on low-price plans;
- prefer a price point where fixed fees do not dominate unit economics.

Do not expose provider fee calculations as financial accounting truth unless they are reconciled from actual provider transaction/payout records.

---

# 12. Tax / accounting boundary

Lemon Squeezy as MoR handles customer-side sales tax/VAT obligations for transactions it is merchant of record for.

That **does not mean CodeLocal's Vietnam income is tax-free**.

Separate concerns:

```text
customer indirect tax / VAT / sales tax
   -> MoR responsibility for covered transactions

income received by CodeLocal seller in Vietnam
   -> Vietnam accounting/tax/legal responsibility
```

Before meaningful volume, obtain Vietnam-specific accounting/tax advice for the actual seller identity (individual, household business, Vietnamese company, etc.). Keep provider statements, payout records, invoices and bank reconciliation evidence.

This document is a product/engineering plan, not tax or legal advice.

---

# 13. Provider abstraction

Target interface conceptually:

```text
BillingProvider
  CreateCheckout(...)
  GetCustomerPortal(...)
  VerifyWebhook(...)
  NormalizeWebhook(...)
  GetSubscription(...)
  CancelSubscription(...)
  ChangePlan(...)
```

Provider-specific IDs belong in adapter/storage fields and must not leak into authorization/business rules.

Internal product code should use stable values such as:

```text
FREE
PRO
TEAM
BUSINESS
```

not:

```text
lemonsqueezy_variant_12345
```

This makes later Lemon Squeezy -> Paddle/Stripe migration possible without rewriting CodeLocal's entire access model.

---

# 14. Reconciliation and failure recovery

Webhooks are the fast path, not the only truth recovery mechanism.

Implement periodic/manual reconciliation:

```text
CodeLocal subscription record
       |
       v
fetch provider subscription
       |
       v
compare normalized state
       |
       +-- same -> no-op
       |
       +-- mismatch -> repair + audit
```

Required operator capabilities:

- inspect billing customer/subscription state;
- replay an accepted webhook safely;
- force provider reconciliation;
- view entitlement derivation reason;
- grant/revoke an explicit admin entitlement with audit evidence;
- never edit raw provider history to "fix" access.

---

# 15. Security requirements

Billing is a protected subsystem, similar to authentication.

- API keys/signing secrets only in server secret storage/environment.
- Never send Lemon Squeezy API key to browser/client/runtime.
- Webhook signing secret server-side only.
- Billing endpoints require authenticated user/org context.
- Team checkout validates organization authority.
- Customer portal link endpoint validates ownership before returning signed URL.
- Idempotency on checkout creation where practical.
- Billing changes recorded in an audit trail.
- Do not log card/payment method secrets.
- Entitlement checks fail closed when authority is genuinely unknown; transient provider outage should not automatically revoke a locally confirmed active subscription.

---

# 16. Implementation phases

## BILL-0 — Merchant readiness — P0

- Website/compliance pages.
- Accurate product description.
- Lemon Squeezy account/store.
- KYC/KYB/tax form/payout setup.
- Live-store approval.

Exit gate: a live CodeLocal store can legally/operationally accept a real international payment and has a verified payout destination.

## BILL-1 — Billing domain — P0

- Provider-neutral models.
- Plan catalog.
- Billing customer/subscription persistence.
- Entitlement authority contract.
- Migrations + focused tests.

Exit gate: no provider-specific state is required by core authorization logic.

## BILL-2 — Lemon Squeezy adapter + checkout — P0

- Secure provider config.
- Plan -> variant mapping.
- Checkout endpoint.
- Trusted customer metadata.
- Success/cancel UX.

Exit gate: test-mode checkout can be created only for the authenticated subject and does not directly grant access.

## BILL-3 — Webhooks + entitlement sync — P0

- Signature verification.
- Idempotent event inbox.
- Subscription normalization.
- Entitlement transitions.
- Duplicate/out-of-order tests.

Exit gate: paid access is driven only by verified normalized billing state.

## BILL-4 — Billing UI / Customer Portal — P1

- Account billing panel.
- Current plan/status/renewal.
- Signed Customer Portal link.
- Upgrade/downgrade/cancel UX policy.

## BILL-5 — Reconciliation / operations — P1

- Manual reconcile endpoint/admin flow.
- Scheduled reconciliation if justified by volume.
- Billing audit/logging.
- Refund/payment-failure support flow.

## BILL-6 — Production proof — P0 release gate

Perform real low-value production transactions before broad launch:

1. international card payment;
2. webhook arrival;
3. Pro entitlement activation;
4. customer portal access;
5. cancellation;
6. entitlement remains until correct paid-period end where policy requires;
7. expiry downgrade;
8. refund behavior;
9. payout appears in configured Vietnam payout account;
10. reconciliation records match provider statement.

Do not mark billing "production complete" until a real payout is proven end-to-end.

## BILL-7 — Scale / provider portability — P2

- Team seat billing.
- Coupons/trials as product requires.
- Invoices/tax ID experience for B2B customers.
- Usage-based billing only after authoritative usage metering exists.
- Paddle adapter if needed.
- Stripe adapter only with a legitimate supported entity/banking arrangement.

---

# 17. Acceptance criteria

CodeLocal international billing is production-ready only when all are true:

- [ ] Vietnam merchant/store verification is approved.
- [ ] Vietnam payout method is verified.
- [ ] Live website has required product/legal/support information.
- [ ] Product description cannot reasonably be mistaken for spyware/surveillance.
- [ ] Provider secrets never reach browser/local clients.
- [ ] Checkout cannot grant entitlement from client redirect alone.
- [ ] Webhook signatures are verified.
- [ ] Duplicate webhooks are idempotent.
- [ ] Out-of-order events do not silently regress valid state.
- [ ] Subscription status is normalized before authorization.
- [ ] Payment failure/grace/cancel/expiry/refund behavior is explicitly tested.
- [ ] Ending a subscription does not delete Project Brain/user-owned project data.
- [ ] Customer can manage subscription/payment method through a safe portal flow.
- [ ] Entitlement reason/source is inspectable.
- [ ] Provider reconciliation can repair missed webhook state.
- [ ] Telemetry queues are not used as billing authority.
- [ ] A real production payment has activated the correct plan.
- [ ] A real cancellation/expiry path has been tested.
- [ ] A real payout has reached the Vietnam payout destination.
- [ ] Gross/tax/fee/net records can be reconciled.
- [ ] Core billing domain can support a second provider without changing product authorization contracts.

---

# 18. Things we deliberately do not do

- fake US/Singapore identity or address;
- open a fake Stripe account;
- build our own card vault;
- store raw card data;
- make browser redirect authoritative;
- couple plan authorization to Lemon Squeezy variant IDs;
- treat MoR as exemption from Vietnam income/accounting obligations;
- introduce usage-based billing before usage measurement becomes authoritative;
- delete user project knowledge when payment stops;
- mix billing secrets into Project Brain/knowledge memory.

---

# 19. Official references verified 2026-08-19

- Lemon Squeezy supported countries: https://docs.lemonsqueezy.com/help/getting-started/supported-countries
- Lemon Squeezy store activation / KYC-KYB review: https://docs.lemonsqueezy.com/help/getting-started/activate-your-store
- Lemon Squeezy prohibited products: https://docs.lemonsqueezy.com/help/getting-started/prohibited-products
- Lemon Squeezy fees: https://docs.lemonsqueezy.com/help/getting-started/fees
- Lemon Squeezy getting paid: https://docs.lemonsqueezy.com/help/getting-started/getting-paid
- Lemon Squeezy Merchant of Record: https://docs.lemonsqueezy.com/help/payments/merchant-of-record
- Lemon Squeezy sales tax/VAT: https://docs.lemonsqueezy.com/help/payments/sales-tax-vat
- Lemon Squeezy webhooks: https://docs.lemonsqueezy.com/guides/developer-guide/webhooks
- Lemon Squeezy Customer Portal: https://docs.lemonsqueezy.com/guides/developer-guide/customer-portal
- Lemon Squeezy developer guide: https://docs.lemonsqueezy.com/guides/developer-guide
- Stripe global availability: https://stripe.com/global

---

# 20. Final implementation rule

**Get approved -> prove payout -> build provider-neutral billing -> verified webhook authority -> entitlement -> self-service portal -> reconciliation -> scale.**

The first commercial goal is not a sophisticated billing platform. It is a reliable end-to-end proof:

```text
international customer pays
        -> CodeLocal activates Pro
        -> subscription remains synchronized
        -> customer can self-manage billing
        -> payout arrives in Vietnam
```

Once that loop works reliably, pricing/growth experiments can begin without rebuilding the financial plumbing.