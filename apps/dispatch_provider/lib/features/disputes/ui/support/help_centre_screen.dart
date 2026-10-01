import 'package:flutter/material.dart';

import '../../../../core/theme/theme_extensions.dart';
import '../../state/rider_dispute_controller.dart';
import '../support/chat_unavailable_screen.dart';
import 'case_list_screen.dart';
import 'faq_category_screen.dart';

// ═══════════════════════════════════════════════════════════════════════════
// Figma 01 — Help Centre
// "How can we help you?" · search · 5 category cards
// Header: back · title · chat icon · My cases
// ═══════════════════════════════════════════════════════════════════════════

class HelpCentreScreen extends StatefulWidget {
  const HelpCentreScreen({super.key, required this.disputeController});

  final RiderDisputeController disputeController;

  @override
  State<HelpCentreScreen> createState() => _HelpCentreScreenState();
}

class _HelpCentreScreenState extends State<HelpCentreScreen> {
  final _searchController = TextEditingController();

  static const _categories = [
    (
      'Deliveries & Pickups',
      'Pickup, delivery, location',
      Icons.local_shipping_outlined,
    ),
    (
      'Earnings & Payments',
      'Payouts, fees, bonuses',
      Icons.account_balance_wallet_outlined,
    ),
    (
      'Account & Verification',
      'Profile, documents, security',
      Icons.verified_user_outlined,
    ),
    (
      'App & Technical Issues',
      'Bugs, performance',
      Icons.phone_android_outlined,
    ),
    (
      'Safety & Conduct',
      'Safety policies, reporting',
      Icons.shield_outlined,
    ),
  ];

  @override
  void dispose() {
    _searchController.dispose();
    super.dispose();
  }

  void _openCategory(String title) {
    Navigator.of(context).push(
      MaterialPageRoute(
        builder: (_) => FaqCategoryScreen(
          categoryTitle: title,
          disputeController: widget.disputeController,
        ),
      ),
    );
  }

  void _openCases() {
    Navigator.of(context).push(
      MaterialPageRoute(
        builder: (_) => CaseListScreen(
          disputeController: widget.disputeController,
        ),
      ),
    );
  }

  /// Chat entry point.
  ///
  /// For now this routes to the "Chat unavailable" screen (Figma 12),
  /// which is the correct UX while support agents are offline.
  ///
  /// When you enable live chat:
  ///   1. Call `widget.disputeController.openSupportChat()`
  ///   2. If it returns a `Complaint`, push `DisputeChatScreen`
  ///   3. If null (error), fall back to `ChatUnavailableScreen`
  void _openChat() {
    Navigator.of(context).push(
      MaterialPageRoute(
        builder: (_) => ChatUnavailableScreen(
          disputeController: widget.disputeController,
        ),
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    final c = context.palette;

    return Scaffold(
      backgroundColor: c.background,
      body: SafeArea(
        child: Column(
          children: [
            // ── Header ──
            Padding(
              padding: const EdgeInsets.fromLTRB(12, 8, 12, 4),
              child: Row(
                children: [
                  GestureDetector(
                    onTap: () => Navigator.of(context).pop(),
                    behavior: HitTestBehavior.opaque,
                    child: Padding(
                      padding: const EdgeInsets.all(8),
                      child: Icon(
                        Icons.arrow_back_rounded,
                        size: 22,
                        color: c.text,
                      ),
                    ),
                  ),
                  const SizedBox(width: 4),
                  Expanded(
                    child: Text(
                      'Support',
                      style: TextStyle(
                        fontSize: 16,
                        fontWeight: FontWeight.w700,
                        color: c.text,
                        letterSpacing: -0.2,
                      ),
                    ),
                  ),
                  // Chat icon
                  GestureDetector(
                    onTap: _openChat,
                    behavior: HitTestBehavior.opaque,
                    child: Padding(
                      padding: const EdgeInsets.all(8),
                      child: Icon(
                        Icons.chat_bubble_outline_rounded,
                        size: 22,
                        color: c.brandTeal500,
                      ),
                    ),
                  ),
                  // My cases
                  GestureDetector(
                    onTap: _openCases,
                    behavior: HitTestBehavior.opaque,
                    child: Padding(
                      padding: const EdgeInsets.all(8),
                      child: Text(
                        'My cases',
                        style: TextStyle(
                          fontSize: 14,
                          fontWeight: FontWeight.w700,
                          color: c.brandTeal500,
                        ),
                      ),
                    ),
                  ),
                ],
              ),
            ),

            Expanded(
              child: ListView(
                padding: const EdgeInsets.fromLTRB(20, 8, 20, 24),
                children: [
                  const SizedBox(height: 4),
                  Text(
                    'How can we help you?',
                    style: TextStyle(
                      fontSize: 26,
                      fontWeight: FontWeight.w800,
                      color: c.text,
                      letterSpacing: -0.4,
                    ),
                  ),
                  const SizedBox(height: 6),
                  Text(
                    'Get help. Resolve faster. Keep moving.',
                    style: TextStyle(
                      fontSize: 15,
                      fontWeight: FontWeight.w500,
                      color: c.textMuted,
                      height: 1.45,
                    ),
                  ),
                  const SizedBox(height: 20),

                  // ── Search ──
                  TextField(
                    controller: _searchController,
                    style: TextStyle(
                      fontSize: 15,
                      fontWeight: FontWeight.w500,
                      color: c.text,
                    ),
                    decoration: InputDecoration(
                      hintText: 'Search for help...',
                      hintStyle: TextStyle(
                        color: c.textDisabled,
                        fontSize: 15,
                        fontWeight: FontWeight.w500,
                      ),
                      prefixIcon: Icon(
                        Icons.search_rounded,
                        size: 22,
                        color: c.textMuted,
                      ),
                      filled: true,
                      fillColor: c.inputFill,
                      contentPadding:
                          const EdgeInsets.symmetric(vertical: 16),
                      border: OutlineInputBorder(
                        borderRadius: BorderRadius.circular(12),
                        borderSide: BorderSide.none,
                      ),
                      enabledBorder: OutlineInputBorder(
                        borderRadius: BorderRadius.circular(12),
                        borderSide: BorderSide.none,
                      ),
                      focusedBorder: OutlineInputBorder(
                        borderRadius: BorderRadius.circular(12),
                        borderSide:
                            BorderSide(color: c.brandTeal500, width: 1.6),
                      ),
                    ),
                  ),
                  const SizedBox(height: 28),

                  // ── Category cards ──
                  for (final cat in _categories) ...[
                    _CategoryCard(
                      title: cat.$1,
                      subtitle: cat.$2,
                      icon: cat.$3,
                      onTap: () => _openCategory(cat.$1),
                    ),
                    const SizedBox(height: 12),
                  ],
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class _CategoryCard extends StatelessWidget {
  const _CategoryCard({
    required this.title,
    required this.subtitle,
    required this.icon,
    required this.onTap,
  });

  final String title;
  final String subtitle;
  final IconData icon;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    return GestureDetector(
      onTap: onTap,
      behavior: HitTestBehavior.opaque,
      child: Container(
        padding: const EdgeInsets.all(16),
        decoration: BoxDecoration(
          color: c.surface,
          borderRadius: BorderRadius.circular(14),
          border: Border.all(color: c.divider),
        ),
        child: Row(
          children: [
            Container(
              width: 44,
              height: 44,
              alignment: Alignment.center,
              decoration: BoxDecoration(
                color: c.teal10,
                borderRadius: BorderRadius.circular(12),
              ),
              child: Icon(icon, size: 22, color: c.brandTeal600),
            ),
            const SizedBox(width: 14),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    title,
                    style: TextStyle(
                      fontSize: 15,
                      fontWeight: FontWeight.w700,
                      color: c.text,
                      letterSpacing: -0.1,
                    ),
                  ),
                  const SizedBox(height: 3),
                  Text(
                    subtitle,
                    style: TextStyle(
                      fontSize: 12.5,
                      fontWeight: FontWeight.w500,
                      color: c.textMuted,
                    ),
                  ),
                ],
              ),
            ),
            Icon(
              Icons.chevron_right_rounded,
              size: 22,
              color: c.textMuted,
            ),
          ],
        ),
      ),
    );
  }
}