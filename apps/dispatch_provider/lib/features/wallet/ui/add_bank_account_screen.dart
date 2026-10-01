import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../../../core/app_mode.dart';
import '../../../core/theme/theme_extensions.dart';
import '../data/wallet_models.dart';
import '../state/wallet_controller.dart';

// ── Nigerian banks (mock — replace with a GET /banks endpoint later) ────

class _Bank {
  const _Bank(this.name, this.code);
  final String name;
  final String code;
}

const _nigerianBanks = <_Bank>[
  _Bank('Access Bank', '044'),
  _Bank('Citibank', '023'),
  _Bank('Ecobank', '050'),
  _Bank('Fidelity Bank', '070'),
  _Bank('First Bank', '011'),
  _Bank('First City Monument Bank', '214'),
  _Bank('Guaranty Trust Bank', '058'),
  _Bank('Heritage Bank', '030'),
  _Bank('Keystone Bank', '082'),
  _Bank('Polaris Bank', '076'),
  _Bank('Providus Bank', '101'),
  _Bank('Stanbic IBTC', '221'),
  _Bank('Standard Chartered', '068'),
  _Bank('Sterling Bank', '232'),
  _Bank('Union Bank', '032'),
  _Bank('United Bank for Africa', '033'),
  _Bank('Unity Bank', '215'),
  _Bank('Wema Bank', '035'),
  _Bank('Zenith Bank', '057'),
];

class AddBankAccountScreen extends StatefulWidget {
  const AddBankAccountScreen({super.key, required this.walletController});

  final WalletController walletController;

  @override
  State<AddBankAccountScreen> createState() => _AddBankAccountScreenState();
}

class _AddBankAccountScreenState extends State<AddBankAccountScreen> {
  final _accountNumberController = TextEditingController();

  _Bank? _selectedBank;
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
      _selectedBank != null;

  Future<void> _onVerify() async {
    setState(() {
      _resolving = true;
      _error = null;
      _resolved = null;
    });

    if (AppModeConfig.instance.isMock) {
      await Future.delayed(const Duration(milliseconds: 700));
      if (!mounted) return;
      setState(() {
        _resolved = BankAccount(
          id: 'mock_resolved',
          bankCode: _selectedBank!.code,
          bankName: _selectedBank!.name,
          accountNumber: _accountNumberController.text.trim(),
          accountName: 'Daniel Okafor',
          currency: 'NGN',
        );
        _resolving = false;
      });
      return;
    }

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
    if (resolved == null) return;

    setState(() {
      _registering = true;
      _error = null;
    });

    if (AppModeConfig.instance.isMock) {
      await Future.delayed(const Duration(milliseconds: 500));
      if (!mounted) return;
      Navigator.of(context).pop(true);
      return;
    }

    final result = await widget.walletController.registerBankAccount(
      bankCode: resolved.bankCode.isNotEmpty
          ? resolved.bankCode
          : _selectedBank!.code,
      bankName: resolved.bankName,
      accountNumber: resolved.accountNumber,
    );
    if (!mounted) return;
    result.when(
      success: (_) => Navigator.of(context).pop(true),
      failure: (err) => setState(() {
        _error = err.message.isNotEmpty
            ? err.message
            : 'Failed to save bank account. Please try again.';
        _registering = false;
      }),
    );
  }

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;

    return Scaffold(
      backgroundColor: colors.background,
      body: SafeArea(
        child: Column(
          children: [
            _buildTopBar(context),
            Expanded(
              child: SingleChildScrollView(
                padding: const EdgeInsets.fromLTRB(20, 12, 20, 24),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.stretch,
                  children: [
                    Text(
                      'Add bank account',
                      style: TextStyle(
                        fontSize: 24,
                        fontWeight: FontWeight.w800,
                        color: colors.text,
                        letterSpacing: -0.5,
                        height: 1.15,
                      ),
                    ),
                    const SizedBox(height: 8),
                    Text(
                      'Link a bank account to receive withdrawals.',
                      style: TextStyle(
                        fontSize: 15,
                        color: colors.textMuted,
                        height: 1.4,
                      ),
                    ),
                    const SizedBox(height: 28),

                    const _FieldLabel('Bank name'),
                    const SizedBox(height: 10),
                    _BankDropdown(
                      value: _selectedBank,
                      onChanged: (v) => setState(() {
                        _selectedBank = v;
                        _resolved = null;
                        _error = null;
                      }),
                    ),
                    const SizedBox(height: 18),

                    const _FieldLabel('Account number'),
                    const SizedBox(height: 10),
                    _AccountField(
                      controller: _accountNumberController,
                      onChanged: (_) => setState(() {
                        _resolved = null;
                        _error = null;
                      }),
                    ),

                    if (_error != null) ...[
                      const SizedBox(height: 14),
                      _ErrorBanner(message: _error!),
                    ],

                    if (_resolved != null) ...[
                      const SizedBox(height: 20),
                      _ResolvedCard(account: _resolved!),
                    ],

                    const SizedBox(height: 28),

                    if (_resolved == null)
                      _PrimaryButton(
                        label: 'Verify account',
                        enabled: _canResolve && !_resolving,
                        isLoading: _resolving,
                        onPressed: _onVerify,
                      )
                    else
                      _PrimaryButton(
                        label: 'Save account',
                        enabled: !_registering,
                        isLoading: _registering,
                        onPressed: _onRegister,
                      ),
                  ],
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }

  Widget _buildTopBar(BuildContext context) {
    final colors = context.palette;
    return Padding(
      padding: const EdgeInsets.fromLTRB(12, 8, 12, 4),
      child: Stack(
        alignment: Alignment.center,
        children: [
          Align(
            alignment: Alignment.centerLeft,
            child: GestureDetector(
              onTap: () => Navigator.of(context).pop(),
              behavior: HitTestBehavior.opaque,
              child: Padding(
                padding: const EdgeInsets.all(8),
                child: Icon(
                  Icons.arrow_back_rounded,
                  size: 22,
                  color: colors.text,
                ),
              ),
            ),
          ),
          Text(
            'Add bank account',
            style: TextStyle(
              fontSize: 17,
              fontWeight: FontWeight.w700,
              color: colors.text,
              letterSpacing: -0.2,
            ),
          ),
          const SizedBox(width: 40),
        ],
      ),
    );
  }
}

// ── Field label ─────────────────────────────────────────────────────

class _FieldLabel extends StatelessWidget {
  const _FieldLabel(this.text);
  final String text;

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;
    return Text(
      text,
      style: TextStyle(
        fontSize: 14,
        fontWeight: FontWeight.w600,
        color: colors.text,
        letterSpacing: -0.1,
      ),
    );
  }
}

// ── Bank dropdown ───────────────────────────────────────────────────

class _BankDropdown extends StatelessWidget {
  const _BankDropdown({required this.value, required this.onChanged});

  final _Bank? value;
  final ValueChanged<_Bank?> onChanged;

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;
    return Container(
      height: 58,
      padding: const EdgeInsets.symmetric(horizontal: 16),
      decoration: BoxDecoration(
        color: colors.surface,
        borderRadius: BorderRadius.circular(12),
        border: Border.all(color: colors.divider, width: 1),
      ),
      child: DropdownButtonHideUnderline(
        child: DropdownButton<_Bank>(
          value: value,
          isExpanded: true,
          icon: Icon(
            Icons.keyboard_arrow_down_rounded,
            color: colors.text,
          ),
          dropdownColor: colors.surface,
          hint: Text(
            'Select your bank',
            style: TextStyle(
              fontSize: 16,
              fontWeight: FontWeight.w400,
              color: colors.textDisabled,
            ),
          ),
          style: TextStyle(
            fontSize: 16,
            fontWeight: FontWeight.w500,
            color: colors.text,
          ),
          items: _nigerianBanks
              .map(
                (b) => DropdownMenuItem<_Bank>(
                  value: b,
                  child: Text(b.name),
                ),
              )
              .toList(),
          onChanged: onChanged,
        ),
      ),
    );
  }
}

// ── Account number field ────────────────────────────────────────────

class _AccountField extends StatelessWidget {
  const _AccountField({required this.controller, required this.onChanged});

  final TextEditingController controller;
  final ValueChanged<String> onChanged;

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;
    return Container(
      decoration: BoxDecoration(
        color: colors.surface,
        borderRadius: BorderRadius.circular(12),
        border: Border.all(color: colors.divider, width: 1),
      ),
      child: TextField(
        controller: controller,
        keyboardType: TextInputType.number,
        maxLength: 10,
        inputFormatters: [FilteringTextInputFormatter.digitsOnly],
        onChanged: onChanged,
        style: TextStyle(
          fontSize: 16,
          fontWeight: FontWeight.w500,
          color: colors.text,
          letterSpacing: 0.5,
        ),
        decoration: InputDecoration(
          hintText: '0123456789',
          hintStyle: TextStyle(
            fontSize: 16,
            fontWeight: FontWeight.w400,
            color: colors.textDisabled,
            letterSpacing: 0.5,
          ),
          border: InputBorder.none,
          counterText: '',
          contentPadding:
              const EdgeInsets.symmetric(horizontal: 16, vertical: 18),
        ),
      ),
    );
  }
}

// ── Error banner ────────────────────────────────────────────────────

class _ErrorBanner extends StatelessWidget {
  const _ErrorBanner({required this.message});
  final String message;

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 12),
      decoration: BoxDecoration(
        color: colors.dangerSoft,
        borderRadius: BorderRadius.circular(12),
        border: Border.all(color: colors.danger, width: 1),
      ),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Icon(
            Icons.error_outline_rounded,
            size: 18,
            color: colors.danger,
          ),
          const SizedBox(width: 10),
          Expanded(
            child: Text(
              message,
              style: TextStyle(
                fontSize: 13,
                fontWeight: FontWeight.w500,
                color: colors.danger,
                height: 1.4,
              ),
            ),
          ),
        ],
      ),
    );
  }
}

// ── Resolved account card ───────────────────────────────────────────

class _ResolvedCard extends StatelessWidget {
  const _ResolvedCard({required this.account});
  final BankAccount account;

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;
    return Container(
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(
        color: colors.teal10,
        borderRadius: BorderRadius.circular(14),
        border: Border.all(color: colors.brandTeal500, width: 1.2),
      ),
      child: Row(
        children: [
          Container(
            width: 40,
            height: 40,
            decoration: BoxDecoration(
              color: colors.brandTeal500,
              shape: BoxShape.circle,
            ),
            child: const Icon(
              Icons.check_rounded,
              size: 22,
              color: Colors.white,
            ),
          ),
          const SizedBox(width: 14),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  account.accountName,
                  style: TextStyle(
                    fontSize: 15,
                    fontWeight: FontWeight.w700,
                    color: colors.text,
                    height: 1.2,
                  ),
                ),
                const SizedBox(height: 4),
                Text(
                  '${account.bankName} · ${account.accountNumber}',
                  style: TextStyle(
                    fontSize: 13,
                    fontWeight: FontWeight.w500,
                    color: colors.textMuted,
                  ),
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }
}

// ── Primary button ──────────────────────────────────────────────────

class _PrimaryButton extends StatelessWidget {
  const _PrimaryButton({
    required this.label,
    required this.enabled,
    required this.isLoading,
    required this.onPressed,
  });

  final String label;
  final bool enabled;
  final bool isLoading;
  final VoidCallback onPressed;

  @override
  Widget build(BuildContext context) {
    return SizedBox(
      width: double.infinity,
      height: 56,
      child: FilledButton(
        onPressed: enabled ? onPressed : null,
        style: FilledButton.styleFrom(
          shape: RoundedRectangleBorder(
            borderRadius: BorderRadius.circular(12),
          ),
          textStyle: const TextStyle(
            fontSize: 16,
            fontWeight: FontWeight.w700,
          ),
        ),
        child: isLoading
            ? const SizedBox(
                width: 22,
                height: 22,
                child: CircularProgressIndicator(
                  color: Colors.white,
                  strokeWidth: 2.2,
                ),
              )
            : Text(label),
      ),
    );
  }
}