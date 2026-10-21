import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../../../core/format/money_format.dart';
import '../../../core/widgets/karryam_ui.dart';
import '../../home/ui/widgets/provider_app_colors.dart';
import '../state/provider_withdrawal_controller.dart';

/// Step 5 — withdrawal success receipt (Figma "Withdrawal Receipt" / Home.png).
class ProviderWithdrawalReceiptScreen extends StatelessWidget {
  const ProviderWithdrawalReceiptScreen({super.key, required this.controller});

  final ProviderWithdrawalController controller;

  @override
  Widget build(BuildContext context) {
    final account = controller.selectedAccount;
    final naira = controller.amountKobo / 100;

    return Scaffold(
      body: Container(
        width: double.infinity,
        decoration: const BoxDecoration(gradient: kProviderBalanceGradient),
        child: SafeArea(
          child: Column(
            children: [
              const Padding(
                padding: EdgeInsets.symmetric(vertical: 18),
                child: Text(
                  'Withdrawal Receipt',
                  style: TextStyle(color: Colors.white, fontSize: 17, fontWeight: FontWeight.w600),
                ),
              ),
              Expanded(
                child: SingleChildScrollView(
                  padding: const EdgeInsets.fromLTRB(20, 8, 20, 24),
                  child: _ReceiptCard(
                    naira: naira,
                    accountName: account?.accountName ?? '',
                    bankName: account?.bankName ?? '',
                    accountNumber: account?.accountNumber ?? '',
                    onDone: () => Navigator.of(context).popUntil((r) => r.isFirst),
                    onDownload: () async {
                      final result = controller.result;
                      final lines = [
                        'Karryam withdrawal receipt',
                        'Amount: ₦${formatNaira(naira)}',
                        if (account != null) 'To: ${account.accountName}, ${account.bankName} ${account.accountNumber}',
                        if (result != null && result.reference.isNotEmpty) 'Reference: ${result.reference}',
                        if (result != null) 'Status: ${result.status}',
                        'Date: ${DateTime.now().toLocal().toString().substring(0, 16)}',
                      ];
                      await Clipboard.setData(ClipboardData(text: lines.join('\n')));
                      if (!context.mounted) return;
                      ScaffoldMessenger.of(context).showSnackBar(
                        const SnackBar(
                          content: Text('Receipt copied. Paste it into WhatsApp, email or notes.'),
                          behavior: SnackBarBehavior.floating,
                        ),
                      );
                    },
                  ),
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}

class _ReceiptCard extends StatelessWidget {
  const _ReceiptCard({
    required this.naira,
    required this.accountName,
    required this.bankName,
    required this.accountNumber,
    required this.onDone,
    required this.onDownload,
  });

  final double naira;
  final String accountName;
  final String bankName;
  final String accountNumber;
  final VoidCallback onDone;
  final VoidCallback onDownload;

  @override
  Widget build(BuildContext context) {
    return ClipPath(
      clipper: _ReceiptClipper(),
      child: Container(
        color: Colors.white,
        padding: const EdgeInsets.fromLTRB(22, 28, 22, 40),
        child: Column(
          children: [
            const _SuccessSeal(),
            const SizedBox(height: 20),
            const Text(
              'Withdrawal Success!',
              style: TextStyle(color: kProviderText, fontSize: 20, fontWeight: FontWeight.w800),
            ),
            const SizedBox(height: 8),
            const Text(
              'Your withdrawal has successfully been processed.',
              textAlign: TextAlign.center,
              style: TextStyle(color: kProviderMuted, fontSize: 13),
            ),
            const SizedBox(height: 24),
            const Text('Total Amount', style: TextStyle(color: kProviderText, fontSize: 13, fontWeight: FontWeight.w700)),
            const SizedBox(height: 6),
            Text(
              '₦ ${formatNaira(naira)}',
              style: const TextStyle(color: kTealText, fontSize: 24, fontWeight: FontWeight.w800),
            ),
            const SizedBox(height: 20),
            const _DashedDivider(),
            const SizedBox(height: 20),
            const Align(
              alignment: Alignment.centerLeft,
              child: Text(
                'Payment To',
                style: TextStyle(color: kProviderText, fontSize: 15, fontWeight: FontWeight.w800),
              ),
            ),
            const SizedBox(height: 12),
            _PaymentToTile(accountName: accountName, bankName: bankName, accountNumber: accountNumber),
            const SizedBox(height: 28),
            KPrimaryButton(label: 'Done', onPressed: onDone),
            const SizedBox(height: 10),
            KSecondaryButton(label: 'Copy Receipt', icon: Icons.copy_rounded, onPressed: onDownload),
          ],
        ),
      ),
    );
  }
}

class _SuccessSeal extends StatelessWidget {
  const _SuccessSeal();

  @override
  Widget build(BuildContext context) {
    return Container(
      width: 96,
      height: 96,
      decoration: const BoxDecoration(color: kTealTint, shape: BoxShape.circle),
      child: Center(
        child: Container(
          width: 64,
          height: 64,
          decoration: const BoxDecoration(color: kTeal, shape: BoxShape.circle),
          child: const Icon(Icons.check_rounded, color: kNavy, size: 38),
        ),
      ),
    );
  }
}

class _PaymentToTile extends StatelessWidget {
  const _PaymentToTile({
    required this.accountName,
    required this.bankName,
    required this.accountNumber,
  });

  final String accountName;
  final String bankName;
  final String accountNumber;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 14),
      decoration: BoxDecoration(
        color: kProviderSurface,
        borderRadius: BorderRadius.circular(12),
      ),
      child: Row(
        children: [
          Container(
            width: 38,
            height: 38,
            decoration: const BoxDecoration(color: kTealTint, shape: BoxShape.circle),
            child: const Icon(Icons.account_balance_rounded, color: kTealText, size: 19),
          ),
          const SizedBox(width: 12),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  accountName.isEmpty ? 'Bank Account' : accountName,
                  style: const TextStyle(color: kProviderText, fontSize: 14, fontWeight: FontWeight.w800),
                ),
                const SizedBox(height: 2),
                Text(
                  bankName,
                  style: const TextStyle(color: kProviderText, fontSize: 13, fontWeight: FontWeight.w600),
                ),
                const SizedBox(height: 1),
                Text(accountNumber, style: const TextStyle(color: kProviderMuted, fontSize: 12.5)),
              ],
            ),
          ),
        ],
      ),
    );
  }
}

class _DashedDivider extends StatelessWidget {
  const _DashedDivider();

  @override
  Widget build(BuildContext context) {
    return LayoutBuilder(
      builder: (context, constraints) {
        const dashWidth = 6.0;
        const dashSpace = 5.0;
        final count = (constraints.maxWidth / (dashWidth + dashSpace)).floor();
        return Row(
          mainAxisAlignment: MainAxisAlignment.spaceBetween,
          children: List.generate(
            count,
            (_) => Container(width: dashWidth, height: 1.5, color: kProviderBorder),
          ),
        );
      },
    );
  }
}

/// Scalloped bottom edge for the receipt card.
class _ReceiptClipper extends CustomClipper<Path> {
  @override
  Path getClip(Size size) {
    const radius = 9.0;
    final path = Path();
    path.lineTo(0, size.height - radius);
    final count = (size.width / (radius * 2)).floor();
    for (var i = 0; i < count; i++) {
      path.arcToPoint(
        Offset((i + 1) * radius * 2, size.height - radius),
        radius: const Radius.circular(radius),
        clockwise: false,
      );
    }
    path.lineTo(size.width, 0);
    path.close();
    return path;
  }

  @override
  bool shouldReclip(covariant CustomClipper<Path> oldClipper) => false;
}
