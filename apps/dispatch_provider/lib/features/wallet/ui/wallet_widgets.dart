import 'package:flutter/material.dart';
import '../../../core/theme/theme_extensions.dart';
import '../../../shared/widgets/account_components.dart';
import '../../../shared/widgets/delivery_collection_widgets.dart';
import '../../../shared/widgets/journey_icon.dart';

String walletAmount(num kobo, String currency) {
  final parts = (kobo / 100).toStringAsFixed(2).split('.');
  final whole = parts.first.replaceAllMapped(
    RegExp(r'(\d)(?=(\d{3})+(?!\d))'),
    (m) => '${m[1]},',
  );
  return '${currency == 'NGN' ? '\u20a6' : '$currency '}$whole.${parts.last}';
}

class WalletEntryCard extends StatelessWidget {
  const WalletEntryCard({super.key, required this.entry, required this.onTap});
  final Map<String, dynamic> entry;
  final VoidCallback onTap;
  @override
  Widget build(BuildContext context) {
    final credit = entry['side'] == 'credit';
    final date = DateTime.tryParse(entry['created_at']?.toString() ?? '');
    final amount = entry['amount_kobo'];
    return DeliveryPanel(
      onTap: onTap,
      padding: const EdgeInsets.all(16),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          AccountEmblem(
            symbol: credit ? JourneySymbol.wallet : JourneySymbol.transfer,
            color: credit ? context.palette.success : context.palette.textMuted,
          ),
          const SizedBox(width: 14),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  entry['memo']?.toString() ?? 'Wallet entry',
                  style: Theme.of(context).textTheme.titleSmall,
                ),
                const SizedBox(height: 6),
                Text(
                  date == null
                      ? 'Date unavailable'
                      : deliveryDate(date, time: true),
                  style: Theme.of(context).textTheme.bodySmall,
                ),
                const SizedBox(height: 12),
                Text(
                  amount is num
                      ? '${credit ? '+' : '-'}${walletAmount(amount, entry['currency']?.toString() ?? 'NGN')}'
                      : 'Amount unavailable',
                  style: Theme.of(context).textTheme.titleMedium?.copyWith(
                    color: credit
                        ? context.palette.success
                        : context.palette.text,
                  ),
                ),
              ],
            ),
          ),
          const Icon(Icons.chevron_right_rounded, size: 20),
        ],
      ),
    );
  }
}
