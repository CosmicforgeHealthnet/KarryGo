import '../../../core/ui/vendor_toast.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import '../../../core/theme/vendor_theme.dart';
import '../../../core/contracts/vendor_contract.dart';
import '../../dashboard/ui/dashboard_format.dart';

int? financeMinor(String value) {
  final text = value.trim();
  if (!RegExp(r'^\d+(\.\d{1,2})?$').hasMatch(text)) return null;
  final parts = text.split('.');
  return int.tryParse(
    '${parts.first}${parts.length == 1 ? '00' : parts.last.padRight(2, '0')}',
  );
}

Color financeStatusColor(Object? status) => switch (status?.toString().toLowerCase()) {
  'paid' || 'active' || 'verified' => KarryamColors.tealInk,
  'failed' || 'reversed' => KarryamColors.error,
  'pending' || 'requires_otp' => const Color(0xFF986013),
  'processing' => const Color(0xFF285EB0),
  _ => KarryamColors.indigo,
};
String financeStatusLabel(Object? status) =>
    status == 'requires_otp' ? 'Verification required' : dashboardLabel(status);
String financeStatusCopy(Object? status) => switch (status) {
  'paid' => 'The server reports this withdrawal as paid.',
  'processing' =>
    'Your transfer is being processed. Refresh to check its current status.',
  'pending' =>
    'Your withdrawal is pending. Refresh to check its current status.',
  'requires_otp' =>
    'The transfer provider requires additional verification. This app cannot complete that verification yet. Refresh to check for an update.',
  'failed' =>
    'The server reports this withdrawal as failed. Refresh your balance before making another request.',
  'reversed' =>
    'The server reports this withdrawal as reversed. Refresh your balance to check the funds available.',
  _ => 'Refresh to check the latest status reported by the server.',
};

class FinanceStatus extends StatelessWidget {
  const FinanceStatus(this.status, {super.key});
  final Object? status;
  @override
  Widget build(BuildContext context) => Container(
    padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 8),
    decoration: BoxDecoration(
      color: (financeStatusColor(
        status,
      ).withValues(alpha: .09)).forSurface(context),
      borderRadius: BorderRadius.circular(12),
    ),
    child: Text(
      financeStatusLabel(status),
      style: TextStyle(
        color: (financeStatusColor(status)).forInk(context),
        fontWeight: FontWeight.w700,
        fontSize: 12,
      ),
    ),
  );
}

class FinanceButton extends StatelessWidget {
  const FinanceButton(
    this.label, {
    super.key,
    this.onPressed,
    this.busy = false,
    this.secondary = false,
    this.icon = Icons.arrow_forward_rounded,
  });
  final String label;
  final VoidCallback? onPressed;
  final bool busy, secondary;
  final IconData icon;
  @override
  Widget build(BuildContext context) => SizedBox(
    width: double.infinity,
    child: FilledButton(
      style: FilledButton.styleFrom(
        minimumSize: const Size(0, 58),
        padding: const EdgeInsets.symmetric(horizontal: 20, vertical: 16),
        backgroundColor: (secondary ? KarryamColors.mint : KarryamColors.teal)
            .forSurface(context),
        foregroundColor: secondary
            ? KarryamColors.tealInk.forInk(context)
            : KarryamColors.navy,
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(20)),
      ),
      onPressed: busy ? null : onPressed,
      child: Row(
        mainAxisAlignment: MainAxisAlignment.center,
        children: [
          if (busy)
            const SizedBox(
              width: 19,
              height: 19,
              child: CircularProgressIndicator(strokeWidth: 2),
            )
          else
            Icon(icon, size: 20),
          const SizedBox(width: 10),
          Flexible(child: Text(label, textAlign: TextAlign.center)),
        ],
      ),
    ),
  );
}

class FinanceCard extends StatelessWidget {
  const FinanceCard({
    super.key,
    required this.child,
    this.color = Colors.white,
  });
  final Widget child;
  final Color color;
  @override
  Widget build(BuildContext context) => Container(
    width: double.infinity,
    padding: const EdgeInsets.all(22),
    decoration: BoxDecoration(
      color: (color).forSurface(context),
      borderRadius: BorderRadius.circular(28),
      border: Border.all(
        color: (KarryamColors.navy.withValues(alpha: .05)).forOutline(context),
      ),
    ),
    child: child,
  );
}

class FinanceNotice extends StatelessWidget {
  const FinanceNotice(this.message, {super.key, this.error = false});
  final String message;
  final bool error;
  @override
  Widget build(BuildContext context) {
    final color = error ? KarryamColors.error : const Color(0xFF986013);
    return Container(
      padding: const EdgeInsets.all(18),
      margin: const EdgeInsets.only(bottom: 16),
      decoration: BoxDecoration(
        color: (color.withValues(alpha: .07)).forSurface(context),
        borderRadius: BorderRadius.circular(20),
      ),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Icon(
            error ? Icons.error_outline_rounded : Icons.info_outline_rounded,
            color: (color).forInk(context),
            size: 22,
          ),
          const SizedBox(width: 12),
          Expanded(
            child: Text(
              message,
              style: TextStyle(color: (color).forInk(context)),
            ),
          ),
        ],
      ),
    );
  }
}

class FinanceTitle extends StatelessWidget {
  const FinanceTitle(this.title, this.subtitle, {super.key});
  final String title, subtitle;
  @override
  Widget build(BuildContext context) => Padding(
    padding: const EdgeInsets.only(bottom: 24),
    child: Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(
          title,
          style: const TextStyle(
            fontSize: 32,
            fontWeight: FontWeight.w700,
            letterSpacing: -1.2,
          ),
        ),
        if (subtitle.isNotEmpty) ...[const SizedBox(height: 8), Text(subtitle)],
      ],
    ),
  );
}

class FinanceFact extends StatelessWidget {
  const FinanceFact(this.label, this.value, {super.key, this.copy = false});
  final String label;
  final Object? value;
  final bool copy;
  @override
  Widget build(BuildContext context) => Padding(
    padding: const EdgeInsets.symmetric(vertical: 10),
    child: Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(
          label,
          style: TextStyle(
            fontSize: 12,
            color: Color(0xFF647780).forInk(context),
          ),
        ),
        const SizedBox(height: 5),
        Row(
          children: [
            Expanded(
              child: SelectableText(
                value?.toString() ?? 'Unavailable',
                style: const TextStyle(fontWeight: FontWeight.w600),
              ),
            ),
            if (copy && value != null)
              IconButton(
                tooltip: 'Copy $label',
                icon: const Icon(Icons.copy_rounded, size: 19),
                onPressed: () async {
                  await Clipboard.setData(
                    ClipboardData(text: value.toString()),
                  );
                  if (context.mounted) {
                    showVendorToast(context, '$label copied');
                  }
                },
              ),
          ],
        ),
      ],
    ),
  );
}

class FinanceEmblem extends StatelessWidget {
  const FinanceEmblem({
    super.key,
    this.color = KarryamColors.tealInk,
    this.size = 88,
  });
  final Color color;
  final double size;
  @override
  Widget build(BuildContext context) => ExcludeSemantics(
    child: SizedBox(
      width: size,
      height: size,
      child: CustomPaint(
        painter: _WalletPainter(
          color,
          Theme.of(context).colorScheme.surfaceContainerHighest,
          color.forInk(context),
        ),
      ),
    ),
  );
}

class _WalletPainter extends CustomPainter {
  _WalletPainter(this.color, this.surface, this.accent);
  final Color surface, accent;
  final Color color;
  @override
  void paint(Canvas canvas, Size size) {
    canvas.save();
    canvas.scale(size.width / 100, size.height / 100);
    final paint = Paint()..color = color.withValues(alpha: .10);
    canvas.drawCircle(const Offset(50, 50), 46, paint);
    canvas.save();
    canvas.translate(50, 47);
    canvas.rotate(-.18);
    canvas.drawRRect(
      RRect.fromRectAndRadius(
        const Rect.fromLTWH(-31, -24, 60, 42),
        const Radius.circular(10),
      ),
      paint..color = color.withValues(alpha: .25),
    );
    canvas.restore();
    canvas.drawRRect(
      RRect.fromRectAndRadius(
        const Rect.fromLTWH(19, 36, 65, 44),
        const Radius.circular(12),
      ),
      paint..color = color,
    );
    canvas.drawRRect(
      RRect.fromRectAndRadius(
        const Rect.fromLTWH(62, 47, 25, 20),
        const Radius.circular(7),
      ),
      paint..color = KarryamColors.teal,
    );
    canvas.drawCircle(
      const Offset(72, 57),
      3,
      paint..color = KarryamColors.navy,
    );
    canvas.drawLine(
      const Offset(30, 48),
      const Offset(47, 48),
      paint
        ..color = Colors.white
        ..strokeWidth = 3
        ..strokeCap = StrokeCap.round,
    );
    canvas.drawCircle(const Offset(77, 21), 12, paint..color = surface);
    canvas.drawLine(
      const Offset(73, 21),
      const Offset(77, 25),
      paint
        ..color = accent
        ..strokeWidth = 2,
    );
    canvas.drawLine(const Offset(77, 25), const Offset(83, 18), paint);
    canvas.restore();
  }

  @override
  bool shouldRepaint(_WalletPainter oldDelegate) =>
      oldDelegate.color != color ||
      oldDelegate.surface != surface ||
      oldDelegate.accent != accent;
}

Future<T?> financeSheet<T>(
  BuildContext context,
  Widget child, {
  Widget? footer,
}) => showModalBottomSheet<T>(
  context: context,
  useSafeArea: true,
  showDragHandle: true,
  isScrollControlled: true,
  backgroundColor: KarryamColors.canvas.forSurface(context),
  builder: (context) => ConstrainedBox(
    constraints: BoxConstraints(
      maxHeight: MediaQuery.sizeOf(context).height * .85,
    ),
    child: Column(
      mainAxisSize: MainAxisSize.min,
      children: [
        Flexible(
          child: SingleChildScrollView(
            padding: EdgeInsets.fromLTRB(
              24,
              0,
              24,
              footer == null
                  ? 24 + MediaQuery.viewInsetsOf(context).bottom
                  : 12,
            ),
            child: child,
          ),
        ),
        if (footer != null)
          Padding(
            padding: EdgeInsets.fromLTRB(
              24,
              12,
              24,
              24 + MediaQuery.viewInsetsOf(context).bottom,
            ),
            child: footer,
          ),
      ],
    ),
  ),
);

Future<void> showFinanceTransaction(BuildContext context, JsonObject value) =>
    financeSheet<void>(
      context,
      Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          const FinanceTitle(
            'Transaction receipt',
            'Recorded wallet activity.',
          ),
          Text(
            dashboardMoney(value['amount_kobo'], value['currency']),
            style: const TextStyle(fontSize: 27, fontWeight: FontWeight.w700),
          ),
          const SizedBox(height: 20),
          FinanceCard(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                FinanceFact('Reference', value['reference'], copy: true),
                FinanceFact('Type', dashboardLabel(value['transaction_type'])),
                FinanceFact('Direction', dashboardLabel(value['side'])),
                FinanceFact('Memo', value['memo']),
                FinanceFact('Recorded at', value['created_at']),
              ],
            ),
          ),
          const SizedBox(height: 20),
          FinanceButton(
            'Done',
            icon: Icons.check_rounded,
            onPressed: () => Navigator.pop(context),
          ),
        ],
      ),
    );
