# Tecora Backend MVP Roadmap

This roadmap is based on the current repository state and the MVP goals:

- Manage customers for organizations
- Create work orders for customers
- When a work order is fulfilled, create a document with a digital signature and store it

The goal here is planning, not implementation. The sequence below is chosen to fit the codebase that already exists: a small Go API, PostgreSQL-backed modules, startup-applied additive migrations, Clerk auth middleware, and admin-scoped CRUD patterns.

## Terminology

To keep this roadmap unambiguous:

- `users` are the app users from Tecora's perspective
- These users are service businesses or operators such as plumbers, gardeners, and other maintenance professionals
- `customers` are the clients those users work for
- Customer management in this roadmap means CRM-style records owned by an organization so Tecora users can manage the people or businesses they serve

## Current Repo Baseline

What already exists:

- App bootstrap, config loading, database connection, and startup migration runner
- Public health endpoint
- Clerk auth middleware and admin allowlist middleware
- `organizations` domain with admin CRUD endpoints
- `users` domain with admin CRUD endpoints and organization linkage
- Embedded SQL migrations with additive schema posture

What does not exist yet:

- CRM-style customer domain for the clients Tecora users serve
- Work order domain
- Document generation pipeline
- Signature capture or signature provider integration
- File/object storage integration
- Organization-scoped non-admin application routes
- Role model beyond admin allowlist

## Recommended Delivery Order

The MVP should be implemented in this order:

1. Tenancy and authorization alignment
2. Customer management
3. Work order management
4. Work order fulfillment workflow
5. Document generation and storage
6. Digital signature capture/finalization

This order makes the most sense because each later milestone depends on the previous one:

- Customer and work-order routes should be organization-scoped, not only admin-global
- Work orders need customers
- Fulfillment needs a stable work order lifecycle
- Documents should be generated from completed work order data
- Digital signatures should be attached to a finalized document flow, not designed in isolation

## Milestone 1: Tenancy and Authorization Alignment

### Outcome

The API has a clear rule for how organization members access organization data, separate from the existing admin-only management routes.

### Why this comes first

The repo already has Clerk auth, an admin allowlist, local users linked to organizations, and an unused org-scope middleware path. Business features should build on a settled authorization model rather than defaulting everything to admin routes and revisiting it later.

### Likely backend scope

- Decide whether organization access is enforced by:
  - Clerk `org_slug` claims
  - local `users` membership records
  - both
- Introduce organization-scoped route groups for non-admin application behavior
- Define who can create, update, fulfill, and retrieve business records
- Keep admin routes for platform-level management concerns

### Design notes

- `users` are Tecora app users, not CRM customers
- Customers should remain a separate business entity from authenticated users
- This milestone may not require a large migration, but it does require a clear routing and permission strategy

### Delegation-ready task slices

- Propose authorization model for org members
- Propose route structure for org-scoped application endpoints
- Decide how Clerk claims and local users interact
- Add lightweight middleware or request-context conventions if needed

## Milestone 2: Customer Management

### Outcome

Organizations can create, list, update, and archive customers.

### Why this comes first

Customers are the first real CRM entity under an organization. This establishes the tenant-scoped application pattern that the rest of the MVP will reuse.

### Likely backend scope

- New `customers` table linked to `organization_id`
- Customer fields such as:
  - `id`
  - `organization_id`
  - `external_ref` or internal customer code if needed later
  - `name`
  - `email`
  - `phone`
  - `address` or service address fields
  - `notes`
  - timestamps and soft delete field
- `internal/customers` package following the same shape as `organizations` and `users`
- Admin routes first, then organization-scoped authenticated routes if needed
- Validation rules for required identity/contact fields

### Design notes

- This is a CRM-style customer model for the clients a Tecora user serves
- Keep the first customer model simple; avoid adding contacts, billing accounts, or multiple addresses unless the product already requires them
- Prefer soft deletion consistency with the current repo
- Add indexes for organization-scoped listing and lookup

### Delegation-ready task slices

- Define customer schema and additive migration
- Implement customer repository
- Implement customer handler and route registration
- Add tests for customer repository and handlers
- Add README and roadmap updates if route conventions change

## Milestone 3: Work Order Management

### Outcome

Organizations can create and manage work orders tied to customers.

### Why this comes second

Work orders are the operational core of the product, but they should reference an already-stable customer record instead of duplicating customer data everywhere.

### Likely backend scope

- New `work_orders` table linked to:
  - `organization_id`
  - `customer_id`
- Core fields such as:
  - `id`
  - `work_order_number` or generated reference
  - `status`
  - `title`
  - `description`
  - `scheduled_for`
  - `fulfilled_at`
  - timestamps and soft delete field
- Optional `work_order_items` or `work_order_tasks` table only if line items are part of the MVP
- CRUD plus status transition endpoints

### Recommended status model

Start with a narrow lifecycle:

- `draft`
- `scheduled`
- `in_progress`
- `fulfilled`
- `cancelled`

### Design notes

- Do not allow document generation to depend on mutable draft data without a clear snapshot strategy
- Add organization ownership checks everywhere; a work order must never cross org boundaries
- Decide early whether work order numbers are globally unique or unique per organization

### Delegation-ready task slices

- Define work order schema and migration
- Implement work order repository and status transition rules
- Implement work order handlers and route registration
- Add tests for cross-org access protections and invalid transitions
- Add reference-number generation strategy

## Milestone 4: Fulfillment Workflow

### Outcome

A work order can move into a completed state with enough captured data to support downstream documents and signatures.

### Why this is separate from work order CRUD

Fulfillment is not just another update. It is the boundary where operational data becomes record data that should be treated as durable and auditable.

### Likely backend scope

- Explicit fulfillment endpoint instead of generic free-form update
- Fulfillment payload may include:
  - completion timestamp
  - completion notes
  - performed work summary
  - technician/operator name or user id
  - customer acknowledgment metadata if collected at completion time
- Snapshot fields if the final document must remain stable even after customer/work order edits

### Design notes

- Treat fulfillment as a one-way business event unless there is a strong reason to reopen jobs
- If documents are derived from fulfillment data, store a stable snapshot or clearly define the source of truth
- Keep this additive and backward-compatible with the current migration model

### Delegation-ready task slices

- Define fulfillment data model changes
- Implement fulfillment service logic and endpoint
- Add transition guards and tests
- Decide which fields are mutable before and after fulfillment

## Milestone 5: Document Generation and Storage

### Outcome

The system can generate a completion document for a fulfilled work order and store it for retrieval.

### Why this comes before signature finalization

You need a stable document lifecycle before deciding how signatures are applied and persisted. Otherwise the signature implementation will get coupled to an unstable rendering flow.

### Likely backend scope

- New `documents` table linked to:
  - `organization_id`
  - `work_order_id`
  - optionally `customer_id`
- Metadata fields such as:
  - document type
  - storage key
  - mime type
  - generation status
  - created timestamp
  - finalized timestamp
- A document generator component that renders a PDF or similar artifact from fulfilled work order data
- Storage abstraction, likely targeting object storage rather than PostgreSQL blobs

### Important product/technical decisions

- Use Cloud Storage for file persistence instead of storing large binaries in Postgres
- Store document metadata and object keys in Postgres
- Generate documents from a stable snapshot of fulfillment data
- Decide whether documents are regenerated or immutable after creation

### Delegation-ready task slices

- Design document metadata schema and migration
- Add storage abstraction and provider wiring
- Implement document generation service
- Implement document retrieval endpoint or signed download flow
- Add tests around document creation metadata and failure handling

## Milestone 6: Digital Signature

### Outcome

A fulfillment document can be signed and the signed result is stored and traceable.

### Why this comes last in MVP sequencing

Signature work touches legal, UX, storage, audit, and document immutability concerns. It should be layered on top of a stable customer, work order, fulfillment, and document model.

### Two viable MVP approaches

#### Option A: Capture a simple drawn signature image

Backend responsibilities:

- Accept signature image or vector payload
- Associate it to a document or fulfillment record
- Stamp it into the final document or store it alongside the document
- Record signer name, signed timestamp, and source metadata

This is faster to ship, but weaker if stronger audit/compliance guarantees are needed.

#### Option B: Use an e-sign provider

Backend responsibilities:

- Create signature request
- receive provider callbacks
- store provider envelope/signature status
- fetch or store final signed artifact

This is better for auditability but introduces provider complexity and integration work.

### Recommendation for MVP

Start with Option A unless legal/compliance requirements already require a third-party e-sign workflow.

### Delegation-ready task slices

- Decide signature model and legal requirements
- Add schema for signature/document status metadata
- Implement signature upload or provider integration
- Implement signed document finalization flow
- Add audit-oriented tests and retrieval behavior

## Cross-Cutting Work Needed Early

These are not standalone product milestones, but they should be addressed while building Milestones 1 and 2.

### 1. Organization-scoped route pattern

The repo currently has admin CRUD patterns and Clerk auth middleware, but the main business features will likely need org-scoped application routes, not only admin routes.

Expected additions:

- a route structure for organization members
- use of Clerk claims org context where appropriate
- authorization checks beyond the admin allowlist

### 2. Authorization model

Current auth is:

- Clerk token verification
- admin allowlist for admin routes

The MVP will likely need at least:

- admin users
- organization members/operators
- CRM customer records owned by those organizations

Before implementing many business routes, decide whether app access is based on:

- Clerk org claims only
- local user records linked to Clerk users
- both

### 3. Shared API patterns

As new modules are added, the codebase will benefit from:

- consistent pagination shape for list endpoints
- common validation conventions
- shared error response patterns
- optional service layer where business logic becomes more than simple CRUD

### 4. Storage and document configuration

Document/signature work will require config for:

- storage bucket or provider
- signed URL/download strategy
- document generation limits and failure handling

## Suggested Phase Plan

### Phase 0: Foundation alignment

- Confirm MVP scope and fields for customers, work orders, fulfillment, and signature
- Decide auth model for organization members
- Decide storage target for generated documents

### Phase 1: Tenancy and authorization

- Implement org-scoped access rules and route conventions

### Phase 2: Customers

- Implement customer schema, CRUD, and tests

### Phase 3: Work orders

- Implement work order schema, CRUD, status model, and tests

### Phase 4: Fulfillment

- Implement fulfillment transition and snapshot decisions

### Phase 5: Documents

- Implement document generation, metadata persistence, and storage integration

### Phase 6: Signatures

- Implement signature capture/finalization and signed document retrieval

## Parallelization Guide

This section is intended to help delegation. Some work can move in parallel, but the core domain model still has a strict dependency chain.

### Work that should stay sequential

These should be treated as gated milestones:

1. Tenancy and authorization alignment
2. Customer domain model
3. Work order domain model
4. Fulfillment model

Reason:

- Customer routes should align with the org-scoped auth model
- Work orders depend on customers
- Fulfillment depends on the work order lifecycle
- Documents and signatures depend on stable fulfillment output

### Work that can run in parallel early

Once Milestone 1 has enough clarity, these can be delegated at the same time:

- Customer schema design
- Customer handler and repository implementation
- Shared API conventions
  - pagination shape
  - validation patterns
  - error response consistency
- Auth middleware refinement for org-scoped routes
- Test scaffolding for new domain packages

### Work that can run in parallel during work orders

Once the customer model is settled, these can run in parallel:

- Work order schema and status model
- Work order repository and handlers
- Work order numbering strategy
- Cross-org authorization test coverage
- Fulfillment payload design draft

The fulfillment payload design can start before full work-order CRUD is finished, but fulfillment implementation should wait until the work-order lifecycle is stable.

### Work that can run in parallel for documents

Once fulfillment data is defined, these can run in parallel:

- Document metadata schema
- Storage abstraction and provider wiring
- PDF generation approach or rendering prototype
- Download/retrieval route design
- Signature model proposal

This is a good place to split architecture work from implementation work.

### Work that can run in parallel for signatures

Once the document lifecycle is defined, these can run in parallel:

- Signature capture payload design
- Signed document finalization flow
- Audit metadata schema
- Retrieval/download behavior for signed artifacts

If you choose a third-party e-sign provider, provider evaluation and webhook contract design can also run in parallel with document storage work.

### Recommended parallel delegation bundles

These are the best parallel tracks for the current roadmap.

#### Bundle A: After tenancy/auth direction is chosen

- Track 1: customer schema and migration draft
- Track 2: customer API design
- Track 3: auth and route middleware adjustments
- Track 4: shared API/testing conventions

#### Bundle B: After customer model is stable

- Track 1: work order schema and migration draft
- Track 2: work order API and lifecycle design
- Track 3: fulfillment payload and snapshot strategy
- Track 4: authorization and cross-org test plan

#### Bundle C: After fulfillment is defined

- Track 1: document metadata schema
- Track 2: storage provider abstraction
- Track 3: PDF generation implementation spike
- Track 4: signature model decision

### Low-risk parallel work across most phases

These can often be delegated without blocking the main domain work:

- README and endpoint documentation updates
- Test coverage expansion
- Request/response contract documentation
- Refactoring shared helpers once patterns repeat

## Suggested First Delegation Backlog

If you want to begin delegating immediately, this is the best order:

1. Auth and org-scoping proposal
   - how non-admin users access organization data
   - whether access is based on Clerk org claims, DB membership, or both
2. Customer domain proposal
   - schema draft
   - route shape
   - validation rules
3. Work order domain proposal
   - schema draft
   - status model
   - lifecycle endpoints
4. Document architecture proposal
   - PDF generation strategy
   - storage strategy
   - signature attachment model

## Biggest Risks To Control Early

- Overdesigning the data model before the first customer and work order flows exist
- Blurring Tecora app users with CRM customers in the domain model
- Mixing mutable operational data with immutable fulfillment/document records
- Introducing document storage before deciding where binary files live
- Building signature capture before the document lifecycle is stable
- Expanding startup migrations into long-running or destructive operations

## Practical Recommendation

The next work item should be a short tenancy and authorization design pass. After that, the first implementation milestone should be Customer Management. That keeps the business routes aligned with the intended org model while still starting with the lowest-risk additive domain under the current migration strategy.
