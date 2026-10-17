import '../../../shared/widgets/account_components.dart';
import '../../../shared/widgets/delivery_collection_widgets.dart';
import '../../../shared/widgets/journey_icon.dart';
import '../../home/ui/widgets/home_feedback.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../../../core/theme/theme_extensions.dart';
import '../../../shared/widgets/async_content.dart';
import '../data/wallet_models.dart';
import '../state/wallet_controller.dart';

class AddBankAccountScreen extends StatefulWidget {
  const AddBankAccountScreen({super.key, required this.walletController});

  final WalletController walletController;

  @override
  State<AddBankAccountScreen> createState() => _AddBankAccountScreenState();
}

class _AddBankAccountScreenState extends State<AddBankAccountScreen> {
  final _accountNumberController = TextEditingController();

  SupportedBank? _selectedBank;
  List<SupportedBank> _banks = [];
  bool _directoryLoading = true;
  String? _directoryError;

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (mounted) _loadDirectory();
    });
  }

  Future<void> _loadDirectory() async {
    setState(() {
      _directoryLoading = true;
      _directoryError = null;
    });
    final result = await widget.walletController.loadSupportedBanks();
    if (!mounted) return;
    setState(() {
      _directoryLoading = false;
      result.when(
        success: (banks) {
          _banks = banks;
          final previous = _selectedBank?.code;
          _selectedBank = banks
              .where((bank) => bank.code == previous)
              .firstOrNull;
          if (_selectedBank == null) _resolved = null;
        },
        failure: (error) {
          _directoryError = error.statusCode == 404
              ? 'Banks are not available on this server yet.'
              : 'Banks could not be loaded. Please try again.';
          _banks = [];
          _selectedBank = null;
          _resolved = null;
        },
      );
    });
  }

  BankAccount? _resolved;
  bool _resolving = false;
  bool _registering = false;
  String? _error;

  @override
  void dispose() {
    _accountNumberController.dispose();
    super.dispose();
  }

  bool get _canResolve =>
      _accountNumberController.text.trim().length == 10 &&
      _selectedBank != null &&
      !_directoryLoading &&
      _directoryError == null;

  Future<void> _onVerify() async {
    if (!_canResolve || _resolving || _registering) return;
    FocusScope.of(context).unfocus();
    setState(() {
      _resolving = true;
      _error = null;
      _resolved = null;
    });

    final result = await widget.walletController.resolveBankAccount(
      accountNumber: _accountNumberController.text.trim(),
      bankCode: _selectedBank!.code,
    );
    if (!mounted) return;
    result.when(
      success: (data) => setState(() {
        _resolved = data;
        _resolving = false;
      }),
      failure: (err) => setState(() {
        _error = err.message.isNotEmpty
            ? err.message
            : 'Could not verify account. Check the details and try again.';
        _resolving = false;
      }),
    );
  }

  Future<void> _onRegister() async {
    final resolved = _resolved;
    if (resolved == null || _registering || _resolving) return;

    setState(() {
      _registering = true;
      _error = null;
    });

    final result = await widget.walletController.registerBankAccount(
      bankCode: resolved.bankCode.isNotEmpty
          ? resolved.bankCode
          : _selectedBank!.code,
      bankName: _selectedBank!.name,
      accountNumber: resolved.accountNumber,
    );
    if (!mounted) return;
    result.when(
      success: (_) async {
        setState(() => _registering = false);
        await showHomeFeedback(
          context,
          title: 'Bank account saved',
          message: 'Your account is ready to use for withdrawals.',
        );
        if (mounted) Navigator.of(context).pop(true);
      },
      failure: (err) => setState(() {
        _error = err.message.isNotEmpty
            ? err.message
            : 'Failed to save bank account. Please try again.';
        _registering = false;
      }),
    );
  }

  Future<void> _chooseBank() async {
    var query = '';
    final bank = await showModalBottomSheet<SupportedBank>(
      context: context,
      useSafeArea: true,
      showDragHandle: true,
      isScrollControlled: true,
      barrierColor: Colors.black.withValues(alpha: .5),
      builder: (sheet) => FractionallySizedBox(
        heightFactor: .85,
        child: StatefulBuilder(
          builder: (context, update) {
            final visible = _banks
                .where(
                  (b) => b.name.toLowerCase().contains(query.toLowerCase()),
                )
                .toList();
            return Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                Padding(
                  padding: const EdgeInsets.fromLTRB(20, 0, 20, 16),
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.stretch,
                    children: [
                      Text(
                        'Choose your bank',
                        style: Theme.of(context).textTheme.titleLarge,
                      ),
                      const SizedBox(height: 16),
                      TextField(
                        onChanged: (v) => update(() => query = v),
                        decoration: const InputDecoration(
                          hintText: 'Search banks',
                          prefixIcon: Icon(Icons.search_rounded),
                        ),
                      ),
                    ],
                  ),
                ),
                Expanded(
                  child: ListView.builder(
                    itemCount: visible.isEmpty ? 1 : visible.length,
                    itemBuilder: (context, i) => visible.isEmpty
                        ? const Padding(
                            padding: EdgeInsets.all(24),
                            child: Text('No matching banks. Try another name.'),
                          )
                        : AccountMenuTile(
                            title: visible[i].name,
                            symbol: JourneySymbol.bank,
                            onTap: () => Navigator.pop(sheet, visible[i]),
                          ),
                  ),
                ),
              ],
            );
          },
        ),
      ),
    );
    if (bank != null && mounted)
      setState(() {
        _selectedBank = bank;
        _resolved = null;
        _error = null;
      });
  }

  @override
  Widget build(BuildContext context) => AccountPage(
    title: 'Add bank account',
    symbol: JourneySymbol.bank,
    description: 'Choose a bank and verify the account holder before saving.',
    bottom: FilledButton(
      onPressed: _resolving || _registering
          ? null
          : _resolved == null
          ? (_canResolve ? _onVerify : null)
          : _onRegister,
      child: _resolving || _registering
          ? const SizedBox.square(
              dimension: 20,
              child: CircularProgressIndicator(strokeWidth: 2),
            )
          : Text(_resolved == null ? 'Verify account' : 'Save account'),
    ),
    children: [
      if (_directoryLoading)
        const SizedBox(height: 100, child: ContentSkeleton(rows: 1))
      else if (_directoryError != null)
        ActionNotice(
          message: _directoryError!,
          isError: true,
          onRetry: _loadDirectory,
        )
      else if (_banks.isEmpty)
        ActionNotice(
          message: 'No supported banks are available right now.',
          onRetry: _loadDirectory,
        )
      else
        DeliveryPanel(
          padding: EdgeInsets.zero,
          child: AccountMenuTile(
            title: _selectedBank?.name ?? 'Choose bank',
            subtitle: _selectedBank == null
                ? 'Search the bank directory'
                : null,
            symbol: JourneySymbol.bank,
            onTap: _resolving || _registering ? null : _chooseBank,
          ),
        ),
      const SizedBox(height: 22),
      TextField(
        controller: _accountNumberController,
        enabled: !_resolving && !_registering,
        keyboardType: TextInputType.number,
        inputFormatters: [
          FilteringTextInputFormatter.digitsOnly,
          LengthLimitingTextInputFormatter(10),
        ],
        decoration: const InputDecoration(
          labelText: 'Account number',
          hintText: '10-digit account number',
        ),
        onChanged: (_) => setState(() {
          _resolved = null;
          _error = null;
        }),
      ),
      if (_error != null) ...[
        const SizedBox(height: 18),
        ActionNotice(message: _error!, isError: true),
      ],
      AnimatedSize(
        duration: const Duration(milliseconds: 240),
        alignment: Alignment.topCenter,
        child: _resolved == null
            ? const SizedBox.shrink()
            : Padding(
                padding: const EdgeInsets.only(top: 22),
                child: DeliveryPanel(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.stretch,
                    children: [
                      AccountMenuTile(
                        title: 'Account verified',
                        symbol: JourneySymbol.shield,
                        color: context.palette.success,
                      ),
                      AccountValue(
                        label: 'Account holder',
                        value: _resolved!.accountName,
                      ),
                      AccountValue(
                        label: 'Account number',
                        value: _resolved!.accountNumber,
                      ),
                    ],
                  ),
                ),
              ),
      ),
    ],
  );
}
