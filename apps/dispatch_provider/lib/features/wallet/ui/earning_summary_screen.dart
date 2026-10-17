import 'package:flutter/material.dart';
import '../../../shared/widgets/account_components.dart';
import '../../../shared/widgets/delivery_collection_widgets.dart';
import '../../../shared/widgets/journey_icon.dart';
import '../../../shared/widgets/async_content.dart';
import '../state/wallet_controller.dart';
import 'wallet_widgets.dart';
import 'transaction_list_screen.dart';

class EarningSummaryScreen extends StatefulWidget {
  final WalletController walletController;

  const EarningSummaryScreen({super.key, required this.walletController});

  @override
  State<EarningSummaryScreen> createState() => _EarningSummaryScreenState();
}

class _EarningSummaryScreenState extends State<EarningSummaryScreen> {
  bool _visible = true;

  WalletController get _controller => widget.walletController;

  @override
  void initState() {
    super.initState();
    _controller.addListener(_onUpdate);
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (mounted) _controller.loadEarnings();
    });
  }

  @override
  void dispose() {
    _controller.removeListener(_onUpdate);
    super.dispose();
  }

  void _onUpdate() {
    if (mounted) setState(() {});
  }

  @override
  Widget build(BuildContext context) {
    final earnings = _controller.earnings;
    String amount(int? value) => !_visible
        ? '\u2022\u2022\u2022\u2022\u2022\u2022'
        : value == null
        ? 'Unavailable'
        : walletAmount(value, earnings!.currency);
    return AccountPage(
      title: 'Earnings',
      symbol: JourneySymbol.earnings,
      description: 'Available and pending funds in your wallet.',
      onRefresh: () async {
        await _controller.loadEarnings();
      },
      actions: [
        IconButton(
          tooltip: _visible ? 'Hide amounts' : 'Show amounts',
          onPressed: () => setState(() => _visible = !_visible),
          icon: Icon(
            _visible
                ? Icons.visibility_outlined
                : Icons.visibility_off_outlined,
          ),
        ),
      ],
      children: [
        if (_controller.isLoading && earnings == null)
          const SizedBox(height: 200, child: ContentSkeleton(rows: 2))
        else ...[
          DeliveryPanel(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                const Text('Available to withdraw'),
                const SizedBox(height: 12),
                Text(
                  amount(earnings?.availableKobo),
                  style: Theme.of(context).textTheme.headlineMedium,
                ),
                const SizedBox(height: 22),
                const Divider(height: 1),
                const SizedBox(height: 12),
                AccountValue(
                  label: 'Pending',
                  value: amount(earnings?.pendingKobo),
                ),
              ],
            ),
          ),
          const SizedBox(height: 18),
        ],
        if (_controller.error != null)
          ActionNotice(
            message: _controller.error!,
            isError: true,
            onRetry: _controller.loadEarnings,
          ),
        const SizedBox(height: 20),
        DeliveryPanel(
          padding: EdgeInsets.zero,
          child: AccountMenuTile(
            title: 'Wallet transactions',
            subtitle: 'Review recorded credits and debits',
            symbol: JourneySymbol.transfer,
            onTap: () => Navigator.of(context).push(
              MaterialPageRoute(builder: (_) => const TransactionListScreen()),
            ),
          ),
        ),
      ],
    );
  }
}
