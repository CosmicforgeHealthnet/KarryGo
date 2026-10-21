import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../../../core/widgets/karryam_ui.dart';
import '../../home/ui/widgets/provider_app_colors.dart';
import '../models/wallet_models.dart';
import '../state/provider_withdrawal_controller.dart';
import 'widgets/wallet_keypad.dart';
import 'widgets/wallet_widgets.dart';

/// Add a payout bank account (Figma "Enter New Bank Details"): pick a bank,
/// enter the account number — the account name resolves automatically
/// (Paystack lookup) into the read-only Account Name field — then Save
/// (Paystack transfer recipient).
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
    await widget.controller.resolveBank(accountNumber: _accountController.text.trim(), bank: _bank!);
  }

  /// Resolves the account name as soon as a bank + 10-digit number are set.
  void _maybeAutoVerify() {
    if (_canVerify && !widget.controller.resolving && widget.controller.resolved == null) {
      _verify();
    }
  }

  Future<void> _save() async {
    FocusScope.of(context).unfocus();
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
      _maybeAutoVerify();
    }
  }

  @override
  Widget build(BuildContext context) {
    final c = widget.controller;
    return Scaffold(
      body: AnimatedBuilder(
        animation: c,
        builder: (context, _) {
          final resolved = c.resolved;
          return SafeArea(
            child: Column(
              children: [
                const WalletFlowAppBar(
                  title: 'Payments & Withdrawals',
                  subtitle: 'Manage your payment Information here.',
                ),
                const SizedBox(height: 18),
                Expanded(
                  child: ListView(
                    padding: const EdgeInsets.symmetric(horizontal: 20),
                    children: [
                      const Text(
                        'Enter New Bank Details',
                        style: TextStyle(color: kProviderText, fontSize: 18, fontWeight: FontWeight.w800),
                      ),
                      const SizedBox(height: 18),
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
                          _maybeAutoVerify();
                        },
                      ),
                      const SizedBox(height: 20),
                      const _FieldLabel('Account Name'),
                      const SizedBox(height: 8),
                      _AccountNameField(
                        name: resolved?.accountName,
                        resolving: c.resolving,
                      ),
                      if (c.bankError != null) ...[
                        const SizedBox(height: 16),
                        _ErrorBox(message: c.bankError!),
                      ],
                    ],
                  ),
                ),
                Padding(
                  padding: const EdgeInsets.fromLTRB(20, 8, 20, 16),
                  child: WalletPrimaryButton(
                    label: 'Save',
                    loading: c.registering,
                    onPressed: resolved != null ? _save : null,
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
      style: const TextStyle(color: kProviderText, fontSize: 13, fontWeight: FontWeight.w600),
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
          color: kCard,
          borderRadius: BorderRadius.circular(12),
          border: Border.all(color: kProviderBorder),
        ),
        child: Row(
          children: [
            Expanded(
              child: Text(
                bank?.name ?? 'Select Bank',
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
        hintText: 'Enter Acct No',
      ),
    );
  }
}

/// Read-only account-name field: hint until the Paystack lookup fills it in;
/// a small spinner while resolving.
class _AccountNameField extends StatelessWidget {
  const _AccountNameField({required this.name, required this.resolving});
  final String? name;
  final bool resolving;

  @override
  Widget build(BuildContext context) {
    return Container(
      height: 56,
      padding: const EdgeInsets.symmetric(horizontal: 16),
      decoration: BoxDecoration(
        color: name == null ? kCanvas : kTealTint,
        borderRadius: BorderRadius.circular(12),
        border: Border.all(color: name == null ? kProviderBorder : kTeal),
      ),
      child: Row(
        children: [
          Expanded(
            child: Text(
              name ?? 'Enter Acct Name',
              style: TextStyle(
                color: name == null ? kProviderMuted : kProviderText,
                fontSize: 15,
                fontWeight: name == null ? FontWeight.w500 : FontWeight.w700,
              ),
            ),
          ),
          if (resolving)
            const SizedBox.square(
              dimension: 18,
              child: CircularProgressIndicator(strokeWidth: 2, color: kProviderGreen),
            )
          else if (name != null)
            const Icon(Icons.check_circle, color: kTealText, size: 20),
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
    return KNotice(message, tone: KTone.danger);
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
                  style: TextStyle(color: kProviderText, fontSize: 17, fontWeight: FontWeight.w700),
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
