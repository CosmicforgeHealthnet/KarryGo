import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../../home/ui/widgets/provider_app_colors.dart';
import '../models/wallet_models.dart';
import '../state/provider_withdrawal_controller.dart';
import 'widgets/wallet_keypad.dart';
import 'widgets/wallet_widgets.dart';

/// Add a payout bank account: pick a bank, enter the account number, verify the
/// account name (Paystack resolve), then save it (Paystack transfer recipient).
class ProviderAddBankAccountScreen extends StatefulWidget {
  const ProviderAddBankAccountScreen({super.key, required this.controller});

  final ProviderWithdrawalController controller;

  @override
  State<ProviderAddBankAccountScreen> createState() => _ProviderAddBankAccountScreenState();
}

class _ProviderAddBankAccountScreenState extends State<ProviderAddBankAccountScreen> {
  NigerianBank? _bank;
  final _accountController = TextEditingController();

  @override
  void initState() {
    super.initState();
    widget.controller.clearBankError();
  }

  @override
  void dispose() {
    _accountController.dispose();
    super.dispose();
  }

  bool get _canVerify => _bank != null && _accountController.text.trim().length == 10;

  Future<void> _verify() async {
    FocusScope.of(context).unfocus();
    await widget.controller.resolveBank(accountNumber: _accountController.text.trim(), bank: _bank!);
  }

  Future<void> _save() async {
    final ok = await widget.controller.registerBank(
      accountNumber: _accountController.text.trim(),
      bank: _bank!,
    );
    if (ok && mounted) Navigator.of(context).pop();
  }

  Future<void> _pickBank() async {
    final selected = await showModalBottomSheet<NigerianBank>(
      context: context,
      backgroundColor: Colors.white,
      shape: const RoundedRectangleBorder(
        borderRadius: BorderRadius.vertical(top: Radius.circular(20)),
      ),
      builder: (_) => _BankPickerSheet(banks: nigerianBanks),
    );
    if (selected != null) {
      setState(() => _bank = selected);
      widget.controller.clearBankError();
    }
  }

  @override
  Widget build(BuildContext context) {
    final c = widget.controller;
    return Scaffold(
      backgroundColor: Colors.white,
      body: AnimatedBuilder(
        animation: c,
        builder: (context, _) {
          final resolved = c.resolved;
          return SafeArea(
            child: Column(
              children: [
                const WalletFlowAppBar(title: 'Add Bank Account'),
                const SizedBox(height: 12),
                Expanded(
                  child: ListView(
                    padding: const EdgeInsets.symmetric(horizontal: 20),
                    children: [
                      const _FieldLabel('Bank'),
                      const SizedBox(height: 8),
                      _BankSelector(bank: _bank, onTap: _pickBank),
                      const SizedBox(height: 20),
                      const _FieldLabel('Account Number'),
                      const SizedBox(height: 8),
                      _AccountNumberField(
                        controller: _accountController,
                        onChanged: (_) {
                          c.clearBankError();
                          setState(() {});
                        },
                      ),
                      if (resolved != null) ...[
                        const SizedBox(height: 16),
                        _ResolvedAccountBox(name: resolved.accountName),
                      ],
                      if (c.bankError != null) ...[
                        const SizedBox(height: 16),
                        _ErrorBox(message: c.bankError!),
                      ],
                    ],
                  ),
                ),
                Padding(
                  padding: const EdgeInsets.fromLTRB(20, 8, 20, 16),
                  child: resolved == null
                      ? WalletPrimaryButton(
                          label: 'Verify Account',
                          loading: c.resolving,
                          onPressed: _canVerify ? _verify : null,
                        )
                      : WalletPrimaryButton(
                          label: 'Save Account',
                          loading: c.registering,
                          onPressed: _save,
                        ),
                ),
              ],
            ),
          );
        },
      ),
    );
  }
}

class _FieldLabel extends StatelessWidget {
  const _FieldLabel(this.text);
  final String text;
  @override
  Widget build(BuildContext context) {
    return Text(
      text,
      style: const TextStyle(color: kProviderText, fontSize: 14, fontWeight: FontWeight.w700),
    );
  }
}

class _BankSelector extends StatelessWidget {
  const _BankSelector({required this.bank, required this.onTap});
  final NigerianBank? bank;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return GestureDetector(
      onTap: onTap,
      child: Container(
        height: 56,
        padding: const EdgeInsets.symmetric(horizontal: 16),
        decoration: BoxDecoration(
          color: const Color(0xFFF1F2F4),
          borderRadius: BorderRadius.circular(12),
        ),
        child: Row(
          children: [
            Expanded(
              child: Text(
                bank?.name ?? 'Select bank',
                style: TextStyle(
                  color: bank == null ? kProviderMuted : kProviderText,
                  fontSize: 15,
                  fontWeight: FontWeight.w500,
                ),
              ),
            ),
            const Icon(Icons.keyboard_arrow_down_rounded, color: kProviderMuted),
          ],
        ),
      ),
    );
  }
}

class _AccountNumberField extends StatelessWidget {
  const _AccountNumberField({required this.controller, required this.onChanged});
  final TextEditingController controller;
  final ValueChanged<String> onChanged;

  @override
  Widget build(BuildContext context) {
    return TextField(
      controller: controller,
      onChanged: onChanged,
      keyboardType: TextInputType.number,
      maxLength: 10,
      inputFormatters: [FilteringTextInputFormatter.digitsOnly],
      style: const TextStyle(color: kProviderText, fontSize: 16, fontWeight: FontWeight.w600),
      decoration: InputDecoration(
        counterText: '',
        hintText: '0123456789',
        hintStyle: const TextStyle(color: kProviderMuted),
        filled: true,
        fillColor: const Color(0xFFF1F2F4),
        contentPadding: const EdgeInsets.symmetric(horizontal: 16, vertical: 16),
        border: OutlineInputBorder(borderRadius: BorderRadius.circular(12), borderSide: BorderSide.none),
      ),
    );
  }
}

class _ResolvedAccountBox extends StatelessWidget {
  const _ResolvedAccountBox({required this.name});
  final String name;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 14),
      decoration: BoxDecoration(
        color: kProviderGreenTint,
        borderRadius: BorderRadius.circular(12),
        border: Border.all(color: kProviderGreenPale),
      ),
      child: Row(
        children: [
          const Icon(Icons.check_circle, color: kProviderGreen, size: 22),
          const SizedBox(width: 12),
          Expanded(
            child: Text(
              name,
              style: const TextStyle(color: kProviderText, fontSize: 15, fontWeight: FontWeight.w700),
            ),
          ),
        ],
      ),
    );
  }
}

class _ErrorBox extends StatelessWidget {
  const _ErrorBox({required this.message});
  final String message;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
      decoration: BoxDecoration(
        color: kProviderRejectBg,
        borderRadius: BorderRadius.circular(12),
      ),
      child: Text(
        message,
        style: const TextStyle(color: kProviderRejectText, fontSize: 13),
      ),
    );
  }
}

class _BankPickerSheet extends StatelessWidget {
  const _BankPickerSheet({required this.banks});
  final List<NigerianBank> banks;

  @override
  Widget build(BuildContext context) {
    return SafeArea(
      child: ConstrainedBox(
        constraints: BoxConstraints(maxHeight: MediaQuery.of(context).size.height * 0.6),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            const Padding(
              padding: EdgeInsets.fromLTRB(20, 18, 20, 8),
              child: Align(
                alignment: Alignment.centerLeft,
                child: Text(
                  'Select bank',
                  style: TextStyle(color: kProviderText, fontSize: 17, fontWeight: FontWeight.w800),
                ),
              ),
            ),
            Flexible(
              child: ListView.separated(
                shrinkWrap: true,
                itemCount: banks.length,
                separatorBuilder: (_, _) => const Divider(height: 1, color: kProviderBorder),
                itemBuilder: (context, i) => ListTile(
                  title: Text(banks[i].name, style: const TextStyle(color: kProviderText, fontSize: 15)),
                  onTap: () => Navigator.of(context).pop(banks[i]),
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }
}
