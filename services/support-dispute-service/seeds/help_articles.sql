-- Dev seed: a handful of published help/FAQ articles served via GET /support/faqs.
-- Idempotent (fixed UUIDs + ON CONFLICT DO NOTHING) so it is safe to re-run.
-- Apply with: psql "$SUPPORT_DISPUTE_DATABASE_URL" -f seeds/help_articles.sql

INSERT INTO help_articles (id, audience, category, title, body, sort_order, is_published) VALUES
  ('11111111-1111-1111-1111-111111111001', 'all', 'payment',
   'How do refunds work?',
   'If a dispute is resolved in your favour, any refund is returned to your Karry Go wallet. Refunds usually appear within minutes.',
   10, true),
  ('11111111-1111-1111-1111-111111111002', 'customer', 'delayed_arrival',
   'My driver is late — what can I do?',
   'You can track your driver live on the trip screen. If they are significantly delayed you can cancel or contact support from the trip.',
   20, true),
  ('11111111-1111-1111-1111-111111111003', 'customer', 'damaged_goods',
   'My package arrived damaged',
   'Open the trip, tap Report an issue, choose Damaged goods, and attach a photo. Our team will review your evidence and follow up.',
   30, true),
  ('11111111-1111-1111-1111-111111111004', 'provider', 'payment',
   'When are my earnings paid out?',
   'Completed trips are added to your available balance. You can withdraw to a registered bank account at any time from the Earnings tab.',
   40, true),
  ('11111111-1111-1111-1111-111111111005', 'all', 'other',
   'How do I contact support?',
   'Open Support from your profile to start a chat with our team, or raise a complaint tied to a specific trip.',
   50, true)
ON CONFLICT (id) DO NOTHING;
