package crm

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// corporateContactPrivacyHandler owns the individual side of a corporate
// relationship.
//
// Section 8G registers through the same PrivacyHandler port Sections 8B-8F use.
// What it does with an instruction is specific to this domain, and the
// distinction is the whole point: a privacy decision is about a PERSON, while a
// corporate customer is an ORGANIZATION. Erasing an individual must stop that
// individual being contactable without deleting the company's account, its
// sites, its cost centres or the commercial history of the relationship — none
// of which is the individual's data to erase.
type corporateContactPrivacyHandler struct{ service *Service }

func (corporateContactPrivacyHandler) Domain() string { return "corporate_contacts" }

// Restrict is a no-op. A restriction limits processing without erasing
// anything, and a corporate contact record is operational relationship data
// rather than marketing reach — Section 8A's eligibility rule already refuses
// to market to a restricted individual.
func (corporateContactPrivacyHandler) Restrict(context.Context, pgx.Tx, string, string) error {
	return nil
}

// MarketingSuppress is likewise a no-op. Withdrawing marketing consent is not
// the same as resigning as a company's procurement contact, and treating it as
// such would silently break a commercial relationship the customer never asked
// to end.
func (corporateContactPrivacyHandler) MarketingSuppress(context.Context, pgx.Tx, string, string) error {
	return nil
}

// Redact makes every corporate contact for the individual non-actionable while
// leaving the organization untouched.
//
// The row survives, flagged. Deleting it would destroy the organization's
// record of who its procurement contact was during a period the vendor may be
// required to account for — corporate history that is not the individual's to
// erase — while the flag removes the person from every actionable list, blocks
// new activity against them, and drops their presentation label.
func (h corporateContactPrivacyHandler) Redact(ctx context.Context, tx pgx.Tx, businessID, profileID string) error {
	rows, err := tx.Query(ctx, `UPDATE corporate_customer_contacts
		SET privacy_suppressed=true, is_primary_contact=false, updated_at=now(), version=version+1
		WHERE business_id=$1 AND customer_profile_id=$2 AND privacy_suppressed=false
		RETURNING id`, businessID, profileID)
	if err != nil {
		return err
	}
	contactIDs, err := collectIDs(rows)
	if err != nil {
		return err
	}
	if len(contactIDs) == 0 {
		return nil
	}
	// Memberships describe where the person sat in the organization. They stop
	// being actionable with them, and survive as evidence of the arrangement.
	_, err = tx.Exec(ctx, `UPDATE corporate_customer_memberships
		SET status='INACTIVE', updated_at=now(), version=version+1
		WHERE business_id=$1 AND corporate_contact_id = ANY($2) AND status='ACTIVE'`, businessID, contactIDs)
	return err
}

// IdentityMerge canonicalizes corporate contact relationships onto the survivor.
//
// Unlike the individual domains, a merge here can produce a genuine collision:
// if both the source and the survivor are already contacts of the same account,
// moving one onto the other would mean two live rows for one human. The
// database refuses that through a partial unique index, so this handler
// resolves it explicitly — the survivor's row stays, the source's row is
// archived, and the source's memberships move across.
func (h corporateContactPrivacyHandler) IdentityMerge(ctx context.Context, tx pgx.Tx, businessID, sourceProfileID, targetProfileID string) error {
	// Accounts where both people are already contacts. The source row here is
	// the duplicate; the survivor's row is the one that stays live.
	rows, err := tx.Query(ctx, `SELECT s.id, t.id, s.is_primary_contact
		FROM corporate_customer_contacts s
		JOIN corporate_customer_contacts t
		  ON t.business_id = s.business_id AND t.corporate_customer_id = s.corporate_customer_id
		 AND t.customer_profile_id = $3 AND t.status <> 'ARCHIVED'
		WHERE s.business_id = $1 AND s.customer_profile_id = $2 AND s.status <> 'ARCHIVED'`,
		businessID, sourceProfileID, targetProfileID)
	if err != nil {
		return err
	}
	type collision struct {
		sourceID, targetID string
		sourcePrimary      bool
	}
	var collisions []collision
	for rows.Next() {
		var item collision
		if err = rows.Scan(&item.sourceID, &item.targetID, &item.sourcePrimary); err != nil {
			rows.Close()
			return err
		}
		collisions = append(collisions, item)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return err
	}

	for _, item := range collisions {
		// The duplicate is archived first, which also releases it from the
		// one-live-contact and one-primary indexes.
		if _, err = tx.Exec(ctx, `UPDATE corporate_customer_contacts
			SET status='ARCHIVED', is_primary_contact=false, archived_at=now(), updated_at=now(), version=version+1
			WHERE business_id=$1 AND id=$2`, businessID, item.sourceID); err != nil {
			return err
		}
		// Memberships follow the person to the surviving relationship. Anything
		// that would duplicate an arrangement the survivor already has is
		// retired rather than written twice.
		if _, err = tx.Exec(ctx, `UPDATE corporate_customer_memberships m
			SET corporate_contact_id=$1, updated_at=now(), version=version+1
			WHERE m.business_id=$2 AND m.corporate_contact_id=$3 AND m.status <> 'ARCHIVED'
			  AND NOT EXISTS (SELECT 1 FROM corporate_customer_memberships d
				WHERE d.business_id=m.business_id AND d.corporate_contact_id=$1
				  AND d.membership_role=m.membership_role AND d.status <> 'ARCHIVED'
				  AND COALESCE(d.corporate_location_id,'') = COALESCE(m.corporate_location_id,'')
				  AND COALESCE(d.cost_center_id,'') = COALESCE(m.cost_center_id,''))`,
			item.targetID, businessID, item.sourceID); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE corporate_customer_memberships
			SET status='ARCHIVED', archived_at=now(), updated_at=now(), version=version+1
			WHERE business_id=$1 AND corporate_contact_id=$2 AND status <> 'ARCHIVED'`,
			businessID, item.sourceID); err != nil {
			return err
		}
		// If the person the vendor had marked primary was the source, the
		// survivor inherits that decision rather than the account silently
		// losing its primary contact.
		if item.sourcePrimary {
			if _, err = tx.Exec(ctx, `UPDATE corporate_customer_contacts
				SET is_primary_contact=true, updated_at=now(), version=version+1
				WHERE business_id=$1 AND id=$2 AND status='ACTIVE'`, businessID, item.targetID); err != nil {
				return err
			}
		}
	}

	// Everywhere the survivor is not already a contact, the relationship simply
	// moves: same account, same role, same memberships, now pointing at the
	// canonical person. original_profile_id is left alone so the history of who
	// this relationship was created against stays readable.
	_, err = tx.Exec(ctx, `UPDATE corporate_customer_contacts
		SET customer_profile_id=$1, updated_at=now(), version=version+1
		WHERE business_id=$2 AND customer_profile_id=$3`, targetProfileID, businessID, sourceProfileID)
	return err
}

func collectIDs(rows pgx.Rows) ([]string, error) {
	defer rows.Close()
	ids := make([]string, 0, 4)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
