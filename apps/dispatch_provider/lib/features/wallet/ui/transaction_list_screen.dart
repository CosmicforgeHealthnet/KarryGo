import 'wallet_widgets.dart';
import '../../../shared/widgets/account_components.dart';
import '../../../shared/widgets/delivery_collection_widgets.dart';
import '../../../shared/widgets/dispatch_screen_frame.dart';
import '../../../shared/widgets/dispatch_empty_state.dart';
import '../../../shared/widgets/journey_icon.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import '../../../core/config.dart';
import '../../../core/network/provider_session.dart';
import '../../../core/theme/theme_extensions.dart';
import '../../../shared/widgets/async_content.dart';
import '../data/wallet_api.dart';

class TransactionListScreen extends StatefulWidget {
  const TransactionListScreen({super.key, this.initialReference});
  final String? initialReference;
  @override
  State<TransactionListScreen> createState() => _TransactionListScreenState();
}

class _TransactionListScreenState extends State<TransactionListScreen> {
  final List<Map<String, dynamic>> _items = [];
  bool _loaded = false;
  bool _busy = false;
  bool _more = false;
  int _page = 0;
  int _filter = 0;
  bool _openedInitial = false;
  String? _error;
  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    if (!_loaded) {
      _loaded = true;
      _load();
    }
  }

  Future<void> _load({bool next = false}) async {
    if (_busy) return;
    final session = ProviderSession.of(context);
    setState(() {
      _busy = true;
      _error = null;
    });
    final api = WalletApi(
      ApiCoreConfig(baseUrl: AppConfig.walletBaseUrl),
      client: session.client,
    );
    final result = await api.listTransactions(
      session.accessToken() ?? '',
      page: next ? _page + 1 : 1,
    );
    if (!mounted) return;
    result.when(
      success: (data) => setState(() {
        if (!next) _items.clear();
        _items.addAll(
          (data['transactions'] as List? ?? [])
              .whereType<Map<String, dynamic>>(),
        );
        _page = (data['page'] as num?)?.toInt() ?? 1;
        _more = data['has_more'] == true;
      }),
      failure: (e) => setState(
        () => _error = e.statusCode == 404
            ? 'Transaction history is not enabled on this server yet.'
            : e.message,
      ),
    );
    setState(() => _busy = false);
    if (!_openedInitial && widget.initialReference != null) {
      _openedInitial = true;
      final item = _items
          .where((entry) => entry['reference'] == widget.initialReference)
          .firstOrNull;
      if (item != null)
        WidgetsBinding.instance.addPostFrameCallback((_) {
          if (mounted) _detail(item);
        });
    }
  }

  String _amount(Map<String, dynamic> item) =>
      '${item['currency']} ${((item['amount_kobo'] as num).toDouble() / 100).toStringAsFixed(2)}';
  String _date(Map<String, dynamic> item) {
    final date = DateTime.tryParse(
      item['created_at']?.toString() ?? '',
    )?.toLocal();
    return date == null
        ? 'Date unavailable'
        : '${date.day}/${date.month}/${date.year} ${date.hour.toString().padLeft(2, '0')}:${date.minute.toString().padLeft(2, '0')}';
  }

  void _detail(Map<String, dynamic> item) {
    final c = context.palette;
    showModalBottomSheet<void>(
      context: context,
      showDragHandle: true,
      useSafeArea: true,
      isScrollControlled: true,
      barrierColor: Colors.black.withValues(alpha: .5),
      builder: (sheet) => SingleChildScrollView(
        padding: const EdgeInsets.fromLTRB(24, 0, 24, 32),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            const Align(
              alignment: Alignment.centerLeft,
              child: AccountEmblem(symbol: JourneySymbol.transfer, size: 60),
            ),
            const SizedBox(height: 20),
            Text(
              item['memo']?.toString() ?? 'Wallet activity',
              style: Theme.of(sheet).textTheme.titleLarge,
            ),
            const SizedBox(height: 20),
            Text(
              _amount(item),
              style: Theme.of(sheet).textTheme.headlineMedium,
            ),
            const SizedBox(height: 18),
            Text(
              'Posted ${item['side']} • ${_date(item)}',
              style: TextStyle(color: c.textMuted),
            ),
            const SizedBox(height: 24),
            const Text('Reference'),
            const SizedBox(height: 8),
            SelectableText(item['reference']?.toString() ?? ''),
            const SizedBox(height: 20),
            OutlinedButton.icon(
              onPressed: () => Clipboard.setData(
                ClipboardData(text: item['reference']?.toString() ?? ''),
              ),
              icon: const Icon(Icons.copy_outlined),
              label: const Text('Copy reference'),
            ),
          ],
        ),
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    final items = _items
        .where(
          (item) =>
              _filter == 0 ||
              item['side'] == (_filter == 1 ? 'credit' : 'debit'),
        )
        .toList();
    return Scaffold(
      appBar: AppBar(
        leading: const DispatchBackButton(),
        title: const Text('Transactions'),
      ),
      body: Column(
        children: [
          Padding(
            padding: const EdgeInsets.symmetric(horizontal: 20, vertical: 12),
            child: Wrap(
              spacing: 8,
              children: [
                for (final entry in [
                  'All',
                  'Credits',
                  'Debits',
                ].asMap().entries)
                  DeliveryFilter(
                    label: entry.value,
                    selected: _filter == entry.key,
                    onTap: () => setState(() => _filter = entry.key),
                  ),
              ],
            ),
          ),
          if (_error != null)
            Padding(
              padding: const EdgeInsets.all(20),
              child: ActionNotice(
                message: _error!,
                isError: true,
                onRetry: _load,
              ),
            ),
          Expanded(
            child: _busy && _items.isEmpty
                ? const ContentSkeleton()
                : RefreshIndicator(
                    onRefresh: _load,
                    child: ListView(
                      padding: const EdgeInsets.all(20),
                      physics: const AlwaysScrollableScrollPhysics(),
                      children: [
                        if (items.isEmpty && !_busy && _error == null)
                          const DispatchEmptyState(
                            story: EmptyStory.wallet,
                            title: 'No matching activity',
                            message:
                                'Confirmed entries appear here. Try another filter to see other activity.',
                          ),
                        for (final item in items)
                          Padding(
                            padding: const EdgeInsets.only(bottom: 12),
                            child: WalletEntryCard(
                              entry: item,
                              onTap: () => _detail(item),
                            ),
                          ),
                        if (_more)
                          TextButton(
                            onPressed: _busy ? null : () => _load(next: true),
                            child: Text(
                              _busy ? 'Loading...' : 'Load earlier activity',
                            ),
                          ),
                      ],
                    ),
                  ),
          ),
        ],
      ),
    );
  }
}
