import 'package:flutter/material.dart';

import '../../../core/theme/theme_extensions.dart';

class QuickActionsSheet extends StatelessWidget {
  const QuickActionsSheet({
    super.key,
    required this.onWithdraw,
    required this.onAddBank,
    required this.onTransactions,
    required this.onEarnings,
    required this.onHelp,
  });

  final VoidCallback onWithdraw;
  final VoidCallback onAddBank;
  final VoidCallback onTransactions;
  final VoidCallback onEarnings;
  final VoidCallback onHelp;

  static Future<void> show(
    BuildContext context, {
    required VoidCallback onWithdraw,
    required VoidCallback onAddBank,
    required VoidCallback onTransactions,
    required VoidCallback onEarnings,
    required VoidCallback onHelp,
  }) {
    final colors = context.palette;
    return showModalBottomSheet(
      context: context,
      backgroundColor: colors.surface,
      shape: const RoundedRectangleBorder(
        borderRadius: BorderRadius.vertical(top: Radius.circular(20)),
      ),
      builder: (_) => QuickActionsSheet(
        onWithdraw: onWithdraw,
        onAddBank: onAddBank,
        onTransactions: onTransactions,
        onEarnings: onEarnings,
        onHelp: onHelp,
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;
    return SafeArea(
      child: Padding(
        padding: const EdgeInsets.fromLTRB(20, 12, 20, 20),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            // drag handle
            Center(
              child: Container(
                width: 40,
                height: 4,
                decoration: BoxDecoration(
                  color: colors.navy20,
                  borderRadius: BorderRadius.circular(2),
                ),
              ),
            ),
            const SizedBox(height: 20),
            Text(
              'Quick actions',
              style: TextStyle(
                fontSize: 20,
                fontWeight: FontWeight.w800,
                color: colors.text,
                letterSpacing: -0.4,
              ),
            ),
            const SizedBox(height: 20),
            _ActionRow(
              icon: Icons.arrow_upward_rounded,
              title: 'Withdraw funds',
              subtitle: 'Transfer to your bank account',
              onTap: () {
                Navigator.pop(context);
                onWithdraw();
              },
            ),
            _ActionRow(
              icon: Icons.add_rounded,
              title: 'Add bank account',
              subtitle: 'Link a bank account',
              onTap: () {
                Navigator.pop(context);
                onAddBank();
              },
            ),
            _ActionRow(
              icon: Icons.swap_horiz_rounded,
              title: 'Transaction history',
              subtitle: 'View all transactions',
              onTap: () {
                Navigator.pop(context);
                onTransactions();
              },
            ),
            _ActionRow(
              icon: Icons.attach_money_rounded,
              title: 'Earnings',
              subtitle: 'View your earnings summary',
              onTap: () {
                Navigator.pop(context);
                onEarnings();
              },
            ),
            _ActionRow(
              icon: Icons.headset_mic_outlined,
              title: 'Help & support',
              subtitle: 'Get wallet help',
              onTap: () {
                Navigator.pop(context);
                onHelp();
              },
              isLast: true,
            ),
          ],
        ),
      ),
    );
  }
}

class _ActionRow extends StatelessWidget {
  const _ActionRow({
    required this.icon,
    required this.title,
    required this.subtitle,
    required this.onTap,
    this.isLast = false,
  });

  final IconData icon;
  final String title;
  final String subtitle;
  final VoidCallback onTap;
  final bool isLast;

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;
    return Column(
      children: [
        GestureDetector(
          onTap: onTap,
          behavior: HitTestBehavior.opaque,
          child: Padding(
            padding: const EdgeInsets.symmetric(vertical: 12),
            child: Row(
              children: [
                Container(
                  width: 42,
                  height: 42,
                  decoration: BoxDecoration(
                    color: colors.teal10,
                    borderRadius: BorderRadius.circular(11),
                  ),
                  child: Icon(
                    icon,
                    size: 20,
                    color: colors.brandTeal500,
                  ),
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
                          color: colors.text,
                          height: 1.2,
                        ),
                      ),
                      const SizedBox(height: 3),
                      Text(
                        subtitle,
                        style: TextStyle(
                          fontSize: 13,
                          fontWeight: FontWeight.w400,
                          color: colors.textMuted,
                          height: 1.3,
                        ),
                      ),
                    ],
                  ),
                ),
                Icon(
                  Icons.chevron_right_rounded,
                  size: 22,
                  color: colors.navy40,
                ),
              ],
            ),
          ),
        ),
        if (!isLast) Divider(height: 1, color: colors.divider),
      ],
    );
  }
}