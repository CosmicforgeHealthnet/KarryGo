import '../../../shared/widgets/journey_icon.dart';
import '../../../shared/widgets/dispatch_screen_frame.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import '../../../core/theme/theme_extensions.dart';
import '../data/wallet_models.dart';

class WithdrawalResultScreen extends StatelessWidget {
  const WithdrawalResultScreen({super.key, required this.withdrawal});
  final Withdrawal withdrawal;
  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    final paid = withdrawal.status == 'paid';
    final failed = ['failed', 'reversed'].contains(withdrawal.status);
    final color = paid
        ? c.success
        : failed
        ? c.danger
        : c.warning;
    final title = paid
        ? 'Withdrawal paid'
        : failed
        ? 'Withdrawal ${withdrawal.status}'
        : 'Withdrawal requested';
    return Scaffold(
      appBar: AppBar(
        leading: const DispatchBackButton(),
        title: const Text('Withdrawal'),
      ),
      body: SafeArea(
        child: ListView(
          padding: const EdgeInsets.all(24),
          children: [
            const SizedBox(height: 24),
            TweenAnimationBuilder<double>(
              tween: Tween(begin: .8, end: 1),
              duration: const Duration(milliseconds: 350),
              curve: Curves.easeOutCubic,
              builder: (_, value, child) =>
                  Transform.scale(scale: value, child: child),
              child: Center(
                child: Container(
                  width: 96,
                  height: 96,
                  decoration: BoxDecoration(
                    color: color.withValues(alpha: .12),
                    borderRadius: BorderRadius.circular(32),
                  ),
                  child: paid
                      ? JourneyIcon(
                          JourneySymbol.shield,
                          size: 48,
                          color: color,
                        )
                      : Icon(
                          failed
                              ? Icons.error_outline_rounded
                              : Icons.schedule_rounded,
                          size: 48,
                          color: color,
                        ),
                ),
              ),
            ),
            const SizedBox(height: 28),
            Text(
              title,
              textAlign: TextAlign.center,
              style: Theme.of(
                context,
              ).textTheme.headlineSmall?.copyWith(fontWeight: FontWeight.w700),
            ),
            const SizedBox(height: 14),
            Text(
              '${withdrawal.currency} ${withdrawal.amountNaira.toStringAsFixed(2)}',
              textAlign: TextAlign.center,
              style: Theme.of(context).textTheme.headlineMedium,
            ),
            const SizedBox(height: 14),
            Text(
              paid
                  ? 'Your payment has been confirmed.'
                  : failed
                  ? 'Check your wallet before making another request.'
                  : 'Your request was received. The bank transfer has not been confirmed yet. You can safely return to the app.',
              textAlign: TextAlign.center,
              style: TextStyle(color: c.textMuted, height: 1.5),
            ),
            const SizedBox(height: 28),
            if (withdrawal.reference.isNotEmpty)
              Card(
                child: ListTile(
                  title: const Text('Reference'),
                  subtitle: SelectableText(withdrawal.reference),
                  trailing: IconButton(
                    tooltip: 'Copy reference',
                    onPressed: () => Clipboard.setData(
                      ClipboardData(text: withdrawal.reference),
                    ),
                    icon: const Icon(Icons.copy_outlined),
                  ),
                ),
              ),
            const SizedBox(height: 24),
            FilledButton(
              onPressed: () =>
                  Navigator.of(context).popUntil((route) => route.isFirst),
              child: const Text('Back to wallet'),
            ),
          ],
        ),
      ),
    );
  }
}
