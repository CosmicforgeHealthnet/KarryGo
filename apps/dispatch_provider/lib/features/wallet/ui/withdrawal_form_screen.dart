import '../../../shared/widgets/account_components.dart';
import '../../../shared/widgets/delivery_collection_widgets.dart';
import '../../../shared/widgets/journey_icon.dart';
import '../../../shared/widgets/dispatch_screen_frame.dart';
import 'package:flutter/material.dart';

import '../../../core/theme/theme_extensions.dart';
import '../../../shared/widgets/numeric_keypad.dart';
import '../data/wallet_models.dart';
import '../state/wallet_controller.dart';
import 'withdrawal_confirm_screen.dart';
import 'add_bank_account_screen.dart';
import '../../../shared/widgets/async_content.dart';

class WithdrawalFormScreen extends StatefulWidget {
  const WithdrawalFormScreen({super.key, required this.walletController});

  final WalletController walletController;

  @override
  State<WithdrawalFormScreen> createState() => _WithdrawalFormScreenState();
}

class _WithdrawalFormScreenState extends State<WithdrawalFormScreen> {
  String _amount = '';
  String? _selectedAccountId;
  bool _banksLoading = true;
  bool _balanceLoading = true;
  String? _banksError;
  String? _balanceError;
  @override
  void initState() {
    super.initState();
    _controller.addListener(_changed);
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (mounted) {
        _loadBanks();
        _loadBalance();
      }
    });
  }

  Future<void> _loadBanks() async {
    setState(() {
      _banksLoading = true;
      _banksError = null;
    });
    final result = await _controller.loadBankAccounts();
    if (!mounted) return;
    result.when(success: (_) {}, failure: (e) => _banksError = e.message);
    setState(() => _banksLoading = false);
  }

  Future<void> _loadBalance() async {
    setState(() {
      _balanceLoading = true;
      _balanceError = null;
    });
    final result = await _controller.loadEarnings();
    if (!mounted) return;
    result.when(success: (_) {}, failure: (e) => _balanceError = e.message);
    setState(() => _balanceLoading = false);
  }

  Future<void> _addBank() async {
    await Navigator.of(context).push(
      MaterialPageRoute(
        builder: (_) => AddBankAccountScreen(walletController: _controller),
      ),
    );
    if (mounted) await _loadBanks();
  }

  Future<void> _chooseBank() async {
    if (_banksLoading || _banksError != null) return;
    final selected = await showModalBottomSheet<String>(
      context: context,
      useSafeArea: true,
      showDragHandle: true,
      isScrollControlled: true,
      builder: (sheet) => FractionallySizedBox(
        heightFactor: 0.65,
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            Padding(
              padding: const EdgeInsets.fromLTRB(24, 0, 24, 16),
              child: Text(
                'Withdrawal account',
                style: Theme.of(sheet).textTheme.titleLarge,
              ),
            ),
            Expanded(
              child: ListView(
                children: [
                  for (final bank in _accounts)
                    ListTile(
                      leading: JourneyIcon(
                        JourneySymbol.bank,
                        color: sheet.palette.brandTeal600,
                      ),
                      title: Text(bank.bankName),
                      subtitle: Text(
                        '${bank.accountName} / ${_maskAccount(bank.accountNumber)}',
                      ),
                      trailing: bank.id == _selectedAccount?.id
                          ? Icon(
                              Icons.check_circle_rounded,
                              color: sheet.palette.brandTeal600,
                            )
                          : null,
                      selected: bank.id == _selectedAccount?.id,
                      onTap: () => Navigator.pop(sheet, bank.id),
                    ),
                  ListTile(
                    leading: const Icon(Icons.add_rounded),
                    title: const Text('Add a bank account'),
                    onTap: () => Navigator.pop(sheet, '__add__'),
                  ),
                ],
              ),
            ),
          ],
        ),
      ),
    );
    if (!mounted || selected == null) return;
    if (selected == '__add__') {
      await _addBank();
      return;
    }
    setState(() => _selectedAccountId = selected);
  }

  static String _maskAccount(String number) => number.length > 4
      ? '**** ${number.substring(number.length - 4)}'
      : number;

  void _changed() {
    if (mounted) setState(() {});
  }

  @override
  void dispose() {
    _controller.removeListener(_changed);
    super.dispose();
  }

  WalletController get _controller => widget.walletController;

  double get _availableBalance {
    return (_controller.earnings?.availableKobo ?? 0) / 100;
  }

  List<BankAccount> get _accounts {
    return _controller.accounts;
  }

  BankAccount? get _selectedAccount {
    final list = _accounts;
    if (list.isEmpty) return null;
    if (_selectedAccountId != null)
      return list.where((a) => a.id == _selectedAccountId).firstOrNull;
    return list.firstWhere((a) => a.isDefault, orElse: () => list.first);
  }

  double get _amountValue => double.tryParse(_amount) ?? 0;

  bool get _isValid =>
      !_banksLoading &&
      !_balanceLoading &&
      _banksError == null &&
      _balanceError == null &&
      _selectedAccount != null &&
      _amountValue > 0 &&
      _amountValue <= _availableBalance;

  bool get _isOverBalance =>
      _amountValue > 0 && _amountValue > _availableBalance;

  void _onKeyTap(String key) {
    setState(() {
      if (key == 'backspace') {
        if (_amount.isNotEmpty) {
          _amount = _amount.substring(0, _amount.length - 1);
        }
      } else if (_amount.length < 9) {
        _amount = (_amount == '0') ? key : _amount + key;
      }
    });
  }

  void _onContinue() {
    if (!_isValid) return;
    Navigator.of(context).push(
      MaterialPageRoute(
        builder: (_) => WithdrawalConfirmScreen(
          walletController: _controller,
          amount: _amountValue,
          bankAccountId: _selectedAccount!.id,
        ),
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    return Scaffold(
      backgroundColor: c.background,
      appBar: AppBar(
        leading: const DispatchBackButton(),
        title: const Text('Withdraw funds'),
      ),
      body: ListView(
        padding: const EdgeInsets.fromLTRB(20, 12, 20, 16),
        children: [
          DeliveryPanel(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                const Row(
                  children: [
                    AccountEmblem(symbol: JourneySymbol.transfer),
                    SizedBox(width: 12),
                    Expanded(child: Text('Enter withdrawal amount')),
                  ],
                ),
                const SizedBox(height: 24),
                Text(
                  _amount.isEmpty
                      ? '\u20a60'
                      : '\u20a6${_formatGrouped(_amountValue)}',
                  style: Theme.of(context).textTheme.headlineLarge?.copyWith(
                    fontSize: 36,
                    color: _isOverBalance ? c.danger : c.text,
                  ),
                ),
                const SizedBox(height: 12),
                Text(
                  _balanceLoading
                      ? 'Loading balance...'
                      : _balanceError != null
                      ? 'Balance unavailable'
                      : 'Available: ${_formatNaira(_availableBalance)}',
                  style: Theme.of(context).textTheme.bodyMedium,
                ),
                if (_isOverBalance && _balanceError == null) ...[
                  const SizedBox(height: 10),
                  Text(
                    'Amount exceeds your available balance.',
                    style: TextStyle(color: c.danger),
                  ),
                ],
              ],
            ),
          ),
          if (_banksError != null) ...[
            const SizedBox(height: 14),
            ActionNotice(
              message: _banksError!,
              isError: true,
              onRetry: _loadBanks,
            ),
          ],
          if (_balanceError != null) ...[
            const SizedBox(height: 14),
            ActionNotice(
              message: _balanceError!,
              isError: true,
              onRetry: _loadBalance,
            ),
          ],
          const SizedBox(height: 24),
          Text('Send to', style: Theme.of(context).textTheme.titleSmall),
          const SizedBox(height: 12),
          _buildBankSelector(),
          const SizedBox(height: 16),
          NumericKeypad(onKeyTap: _onKeyTap),
        ],
      ),
      bottomNavigationBar: _buildBottomButton(),
    );
  }

  Widget _buildBankSelector() {
    final account = _selectedAccount;

    if (_banksLoading)
      return const SizedBox(height: 80, child: ContentSkeleton());
    if (_banksError != null) return const SizedBox.shrink();
    return DeliveryPanel(
      padding: EdgeInsets.zero,
      child: AccountMenuTile(
        title: account?.bankName ?? 'Add a bank account',
        symbol: JourneySymbol.bank,
        subtitle: account == null
            ? 'Choose where to receive your withdrawal'
            : '${account.accountName} / ${_maskAccount(account.accountNumber)}',
        onTap: account == null ? _addBank : _chooseBank,
      ),
    );
  }

  Widget _buildBottomButton() {
    return SafeArea(
      top: false,
      child: Padding(
        padding: const EdgeInsets.fromLTRB(20, 0, 20, 16),
        child: SizedBox(
          width: double.infinity,
          height: 56,
          child: FilledButton(
            onPressed: _isValid ? _onContinue : null,
            style: FilledButton.styleFrom(
              shape: RoundedRectangleBorder(
                borderRadius: BorderRadius.circular(14),
              ),
              textStyle: const TextStyle(
                fontSize: 16,
                fontWeight: FontWeight.w700,
              ),
            ),
            child: const Text('Continue'),
          ),
        ),
      ),
    );
  }

  static String _formatNaira(double value) {
    final parts = value.toStringAsFixed(2).split('.');
    return '₦${_formatGrouped(double.parse(parts[0]))}.${parts[1]}';
  }

  static String _formatGrouped(double v) {
    final whole = v.toStringAsFixed(0);
    final buffer = StringBuffer();
    for (int i = 0; i < whole.length; i++) {
      if (i != 0 && (whole.length - i) % 3 == 0) buffer.write(',');
      buffer.write(whole[i]);
    }
    return buffer.toString();
  }
}
