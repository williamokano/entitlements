// Package postgres is the billing module's driven adapter: the invoice, line
// item, credit note, and number-sequence repository over the platform pgx pool,
// routing through postgres.Q so writes join the ambient UnitOfWork transaction.
package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/williamokano/entitlements/internal/modules/billing/internal/domain"
	"github.com/williamokano/entitlements/internal/platform/apperr"
	platformpg "github.com/williamokano/entitlements/internal/platform/postgres"
)

// Repo is a Postgres-backed domain.Repository.
type Repo struct {
	pool *pgxpool.Pool
}

// New builds a Repo.
func New(pool *pgxpool.Pool) *Repo { return &Repo{pool: pool} }

// NextNumber atomically allocates the next per-tenant number for kind. The
// upsert takes a row lock on the (tenant, kind) counter, so concurrent issuers
// serialize on it: the returned numbers are 1,2,3… gapless and dup-free, and a
// rolled-back transaction rolls back its increment (no gap). It must run inside
// the issuing transaction (via postgres.Q).
func (r *Repo) NextNumber(ctx context.Context, tenantID uuid.UUID, kind string) (int64, error) {
	// First call for a (tenant, kind) inserts next_number=1 and returns 1; every
	// later call hits the conflict, increments, and returns the new value.
	var n int64
	err := platformpg.Q(ctx, r.pool).QueryRow(ctx,
		`INSERT INTO billing.number_sequences (tenant_id, kind, next_number)
		 VALUES ($1, $2, 1)
		 ON CONFLICT (tenant_id, kind)
		 DO UPDATE SET next_number = billing.number_sequences.next_number + 1
		 RETURNING next_number`,
		tenantID, kind).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("billing: next number: %w", err)
	}
	return n, nil
}

// CreateInvoice inserts an invoice and its line items.
func (r *Repo) CreateInvoice(ctx context.Context, inv *domain.Invoice) error {
	q := platformpg.Q(ctx, r.pool)
	_, err := q.Exec(ctx,
		`INSERT INTO billing.invoices
			(id, tenant_id, subscription_id, number, status, currency, subtotal_minor, tax_minor, total_minor, issued_at, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
		inv.ID, inv.TenantID, nilUUID(inv.SubscriptionID), inv.Number, string(inv.Status), inv.Currency,
		inv.SubtotalMinor, inv.TaxMinor, inv.TotalMinor, inv.IssuedAt, inv.CreatedAt, inv.UpdatedAt)
	if err != nil {
		return fmt.Errorf("billing: insert invoice: %w", err)
	}
	for _, li := range inv.Lines {
		_, err := q.Exec(ctx,
			`INSERT INTO billing.invoice_line_items
				(id, invoice_id, kind, description, plan_or_addon_key, version, unit_price_minor, quantity, amount_minor, currency, position)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
			li.ID, inv.ID, string(li.Kind), li.Description, li.Key, li.Version,
			li.UnitPriceMinor, li.Quantity, li.Amount(), li.Currency, li.Position)
		if err != nil {
			return fmt.Errorf("billing: insert line item: %w", err)
		}
	}
	return nil
}

// UpdateInvoice persists an invoice's mutable fields (status, issued_at, totals).
func (r *Repo) UpdateInvoice(ctx context.Context, inv *domain.Invoice) error {
	_, err := platformpg.Q(ctx, r.pool).Exec(ctx,
		`UPDATE billing.invoices
		 SET status = $2, subtotal_minor = $3, tax_minor = $4, total_minor = $5, issued_at = $6, updated_at = $7
		 WHERE id = $1`,
		inv.ID, string(inv.Status), inv.SubtotalMinor, inv.TaxMinor, inv.TotalMinor, inv.IssuedAt, inv.UpdatedAt)
	if err != nil {
		return fmt.Errorf("billing: update invoice: %w", err)
	}
	return nil
}

const invoiceColumns = `id, tenant_id, subscription_id, number, status, currency, subtotal_minor, tax_minor, total_minor, issued_at, created_at, updated_at`

// GetInvoice returns one invoice with its line items, scoped to the tenant.
func (r *Repo) GetInvoice(ctx context.Context, tenantID, id uuid.UUID) (*domain.Invoice, error) {
	row := platformpg.Q(ctx, r.pool).QueryRow(ctx,
		`SELECT `+invoiceColumns+` FROM billing.invoices WHERE tenant_id = $1 AND id = $2`, tenantID, id)
	inv, err := scanInvoice(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.NotFound("invoice not found")
	}
	if err != nil {
		return nil, fmt.Errorf("billing: get invoice: %w", err)
	}
	lines, err := r.listLines(ctx, inv.ID)
	if err != nil {
		return nil, err
	}
	inv.Lines = lines
	return inv, nil
}

// ListInvoices returns a tenant's invoices, highest number first, each with its
// line items.
func (r *Repo) ListInvoices(ctx context.Context, tenantID uuid.UUID) ([]*domain.Invoice, error) {
	rows, err := platformpg.Q(ctx, r.pool).Query(ctx,
		`SELECT `+invoiceColumns+` FROM billing.invoices WHERE tenant_id = $1 ORDER BY number DESC`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("billing: list invoices: %w", err)
	}
	defer rows.Close()

	var out []*domain.Invoice
	for rows.Next() {
		inv, err := scanInvoice(rows)
		if err != nil {
			return nil, fmt.Errorf("billing: scan invoice: %w", err)
		}
		out = append(out, inv)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("billing: iterate invoices: %w", err)
	}
	for _, inv := range out {
		lines, err := r.listLines(ctx, inv.ID)
		if err != nil {
			return nil, err
		}
		inv.Lines = lines
	}
	return out, nil
}

func (r *Repo) listLines(ctx context.Context, invoiceID uuid.UUID) ([]domain.LineItem, error) {
	rows, err := platformpg.Q(ctx, r.pool).Query(ctx,
		`SELECT id, kind, description, plan_or_addon_key, version, unit_price_minor, quantity, currency, position
		 FROM billing.invoice_line_items WHERE invoice_id = $1 ORDER BY position`, invoiceID)
	if err != nil {
		return nil, fmt.Errorf("billing: list line items: %w", err)
	}
	defer rows.Close()

	var out []domain.LineItem
	for rows.Next() {
		var (
			li   domain.LineItem
			kind string
		)
		if err := rows.Scan(&li.ID, &kind, &li.Description, &li.Key, &li.Version,
			&li.UnitPriceMinor, &li.Quantity, &li.Currency, &li.Position); err != nil {
			return nil, fmt.Errorf("billing: scan line item: %w", err)
		}
		li.Kind = domain.LineKind(kind)
		out = append(out, li)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("billing: iterate line items: %w", err)
	}
	return out, nil
}

// CreateCreditNote inserts a credit note.
func (r *Repo) CreateCreditNote(ctx context.Context, cn *domain.CreditNote) error {
	_, err := platformpg.Q(ctx, r.pool).Exec(ctx,
		`INSERT INTO billing.credit_notes (id, invoice_id, tenant_id, number, amount_minor, currency, reason, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		cn.ID, cn.InvoiceID, cn.TenantID, cn.Number, cn.AmountMinor, cn.Currency, cn.Reason, cn.CreatedAt)
	if err != nil {
		return fmt.Errorf("billing: insert credit note: %w", err)
	}
	return nil
}

// ListCreditNotes returns an invoice's credit notes, oldest number first.
func (r *Repo) ListCreditNotes(ctx context.Context, tenantID, invoiceID uuid.UUID) ([]*domain.CreditNote, error) {
	rows, err := platformpg.Q(ctx, r.pool).Query(ctx,
		`SELECT id, invoice_id, tenant_id, number, amount_minor, currency, reason, created_at
		 FROM billing.credit_notes WHERE tenant_id = $1 AND invoice_id = $2 ORDER BY number`, tenantID, invoiceID)
	if err != nil {
		return nil, fmt.Errorf("billing: list credit notes: %w", err)
	}
	defer rows.Close()

	var out []*domain.CreditNote
	for rows.Next() {
		var cn domain.CreditNote
		if err := rows.Scan(&cn.ID, &cn.InvoiceID, &cn.TenantID, &cn.Number, &cn.AmountMinor, &cn.Currency, &cn.Reason, &cn.CreatedAt); err != nil {
			return nil, fmt.Errorf("billing: scan credit note: %w", err)
		}
		out = append(out, &cn)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("billing: iterate credit notes: %w", err)
	}
	return out, nil
}

// CreatePaymentMethod inserts a tokenized payment method. The DB CHECK
// constraint (schema) rejects a raw PAN even if a caller bypasses the domain
// guard, so credentials never persist.
func (r *Repo) CreatePaymentMethod(ctx context.Context, pm *domain.PaymentMethod) error {
	_, err := platformpg.Q(ctx, r.pool).Exec(ctx,
		`INSERT INTO billing.payment_methods (id, tenant_id, customer_ref, token, brand, last4, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		pm.ID, pm.TenantID, pm.CustomerRef, pm.Token, nullString(pm.Brand), nullString(pm.Last4), pm.CreatedAt)
	if err != nil {
		return fmt.Errorf("billing: insert payment method: %w", err)
	}
	return nil
}

// ListPaymentMethods returns a tenant's stored payment methods, newest first.
func (r *Repo) ListPaymentMethods(ctx context.Context, tenantID uuid.UUID) ([]*domain.PaymentMethod, error) {
	rows, err := platformpg.Q(ctx, r.pool).Query(ctx,
		`SELECT id, tenant_id, customer_ref, token, COALESCE(brand, ''), COALESCE(last4, ''), created_at
		 FROM billing.payment_methods WHERE tenant_id = $1 ORDER BY created_at DESC`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("billing: list payment methods: %w", err)
	}
	defer rows.Close()

	var out []*domain.PaymentMethod
	for rows.Next() {
		var pm domain.PaymentMethod
		if err := rows.Scan(&pm.ID, &pm.TenantID, &pm.CustomerRef, &pm.Token, &pm.Brand, &pm.Last4, &pm.CreatedAt); err != nil {
			return nil, fmt.Errorf("billing: scan payment method: %w", err)
		}
		out = append(out, &pm)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("billing: iterate payment methods: %w", err)
	}
	return out, nil
}

// nullString maps an empty string to a SQL NULL for nullable text columns.
func nullString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanInvoice(row rowScanner) (*domain.Invoice, error) {
	var (
		inv    domain.Invoice
		status string
		subID  *uuid.UUID
	)
	if err := row.Scan(&inv.ID, &inv.TenantID, &subID, &inv.Number, &status, &inv.Currency,
		&inv.SubtotalMinor, &inv.TaxMinor, &inv.TotalMinor, &inv.IssuedAt, &inv.CreatedAt, &inv.UpdatedAt); err != nil {
		return nil, err
	}
	inv.Status = domain.Status(status)
	if subID != nil {
		inv.SubscriptionID = *subID
	}
	return &inv, nil
}

// nilUUID maps the zero UUID to a SQL NULL for the nullable subscription_id.
func nilUUID(id uuid.UUID) *uuid.UUID {
	if id == uuid.Nil {
		return nil
	}
	return &id
}
