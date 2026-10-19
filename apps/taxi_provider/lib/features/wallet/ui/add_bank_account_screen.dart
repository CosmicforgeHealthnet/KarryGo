import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../../../core/theme/karryam_theme.dart';
import '../../../core/widgets/karryam_ui.dart';
import '../data/wallet_api.dart';

/// Adds a payout account. The account name is looked up from the bank so
/// the driver can confirm it before saving. Pops `true` when saved.
class AddBankAccountScreen extends StatefulWidget {
  const AddBankAccountScreen({super.key, required this.api});
  final WalletApi api;

  @override
  State<AddBankAccountScreen> createState() => _AddBankAccountScreenState();
}

class _AddBankAccountScreenState extends State<AddBankAccountScreen> {
  final _number = TextEditingController();
  NigerianBank? _bank;
  String? _accountName;
  bool _resolving = false;
  bool _saving = false;
  String? _error;

  @override
  void initState() {
    super.initState();
    _number.addListener(_maybeResolve);
  }

  @override
  void dispose() {
    _number.dispose();
    super.dispose();
  }

  Future<void> _maybeResolve() async {
    final number = _number.text.trim();
    final bank = _bank;
    if (bank == null || number.length != 10) {
      if (_accountName != null) setState(() => _accountName = null);
      return;
    }
    setState(() {
      _resolving = true;
      _error = null;
    });
    try {
      final name = await widget.api.resolveAccountName(
        bankCode: bank.code,
        accountNumber: number,
      );
      if (!mounted || number != _number.text.trim() || bank != _bank) return;
      setState(() => _accountName = name);
    } on ApiException catch (e) {
      if (mounted) setState(() => _error = e.message);
    } finally {
      if (mounted) setState(() => _resolving = false);
    }
  }

  Future<void> _save() async {
    setState(() {
      _saving = true;
      _error = null;
    });
    try {
      await widget.api.addBankAccount(
        bank: _bank!,
        accountNumber: _number.text.trim(),
      );
      if (mounted) Navigator.of(context).pop(true);
    } on ApiException catch (e) {
      if (mounted) setState(() => _error = e.message);
    } finally {
      if (mounted) setState(() => _saving = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final colors = KarryamColors.of(context);
    return KarryamPage(
      title: 'Add bank account',
      subtitle: 'Where your withdrawals are paid',
      bottom: KarryamButton(
        label: 'Save account',
        busy: _saving,
        onPressed: _accountName == null ? null : _save,
      ),
      body: ListView(
        padding: const EdgeInsets.all(20),
        children: [
          DropdownButtonFormField<NigerianBank>(
            initialValue: _bank,
            isExpanded: true,
            decoration: InputDecoration(
              labelText: 'Bank',
              filled: true,
              fillColor: colors.card,
              border: OutlineInputBorder(
                borderRadius: BorderRadius.circular(12),
              ),
            ),
            items: [
              for (final bank in nigerianBanks)
                DropdownMenuItem(value: bank, child: Text(bank.name)),
            ],
            onChanged: (bank) {
              setState(() => _bank = bank);
              _maybeResolve();
            },
          ),
          const SizedBox(height: 14),
          TextField(
            controller: _number,
            keyboardType: TextInputType.number,
            maxLength: 10,
            inputFormatters: [FilteringTextInputFormatter.digitsOnly],
            decoration: InputDecoration(
              labelText: 'Account number',
              filled: true,
              fillColor: colors.card,
              border: OutlineInputBorder(
                borderRadius: BorderRadius.circular(12),
              ),
            ),
          ),
          const SizedBox(height: 8),
          if (_resolving)
            const Row(
              children: [
                SizedBox(
                  width: 16,
                  height: 16,
                  child: CircularProgressIndicator(strokeWidth: 2),
                ),
                SizedBox(width: 8),
                Text('Checking the account…'),
              ],
            )
          else if (_accountName != null)
            KarryamPanel(
              child: Row(
                children: [
                  Icon(Icons.verified_rounded, color: colors.primary),
                  const SizedBox(width: 10),
                  Expanded(
                    child: Text(
                      _accountName!,
                      style: const TextStyle(fontWeight: FontWeight.w600),
                    ),
                  ),
                ],
              ),
            ),
          if (_error != null) ...[
            const SizedBox(height: 12),
            Text(_error!, style: TextStyle(color: colors.danger)),
          ],
        ],
      ),
    );
  }
}
