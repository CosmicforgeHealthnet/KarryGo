import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../../../app/vendor_runtime.dart';
import '../../../core/contracts/vendor_contract.dart';
import '../../../core/network/vendor_command.dart';
import '../../auth/models/vendor_session.dart';
import '../../onboarding/models/onboarding_models.dart';
import '../state/finance_workspace_controller.dart';
import 'finance_design.dart';

/// A Nigerian bank the merchant can pick by name. The code is what the
/// payout authority (Paystack via payment-wallet) resolves against.
class NigerianBank {
  const NigerianBank(this.name, this.code);
  final String name, code;
}

const nigerianBanks = <NigerianBank>[
  NigerianBank('Access Bank', '044'),
  NigerianBank('Citibank', '023'),
  NigerianBank('Ecobank', '050'),
  NigerianBank('Fidelity Bank', '070'),
  NigerianBank('First Bank of Nigeria', '011'),
  NigerianBank('First City Monument Bank (FCMB)', '214'),
  NigerianBank('Globus Bank', '00103'),
  NigerianBank('Guaranty Trust Bank (GTBank)', '058'),
  NigerianBank('Heritage Bank', '030'),
  NigerianBank('Jaiz Bank', '301'),
  NigerianBank('Keystone Bank', '082'),
  NigerianBank('Kuda Bank', '50211'),
  NigerianBank('Moniepoint MFB', '50515'),
  NigerianBank('OPay', '999992'),
  NigerianBank('PalmPay', '999991'),
  NigerianBank('Polaris Bank', '076'),
  NigerianBank('Providus Bank', '101'),
  NigerianBank('Stanbic IBTC Bank', '221'),
  NigerianBank('Standard Chartered Bank', '068'),
  NigerianBank('Sterling Bank', '232'),
  NigerianBank('Titan Trust Bank', '102'),
  NigerianBank('Union Bank', '032'),
  NigerianBank('United Bank for Africa (UBA)', '033'),
  NigerianBank('Unity Bank', '215'),
  NigerianBank('Wema Bank', '035'),
  NigerianBank('Zenith Bank', '057'),
];

String bankNameForCode(Object? code) {
  final value = code?.toString() ?? '';
  for (final bank in nigerianBanks) {
    if (bank.code == value) return bank.name;
  }
  return value.isEmpty ? 'Bank' : 'Bank ($value)';
}

/// Opens the payout-account form. Returns true when an account was saved.
Future<bool> openPayoutAccount(BuildContext context, VendorRuntime runtime) async {
  final saved = await Navigator.of(context).push<bool>(
    MaterialPageRoute(builder: (_) => BankAccountScreen(runtime: runtime)),
  );
  return saved == true;
}

class BankAccountScreen extends StatefulWidget {
  const BankAccountScreen({super.key, required this.runtime});
  final VendorRuntime runtime;
  @override
  State<BankAccountScreen> createState() => _BankAccountScreenState();
}

class _BankAccountScreenState extends State<BankAccountScreen> {
  final _account = TextEditingController();
  final _code = TextEditingController();
  NigerianBank? _bank;
  JsonObject _profile = {};
  bool _loading = true, _busy = false;
  String? _error, _loadError;
  VendorChallenge? _challenge;
  DateTime? _challengeExpires;
  String? _commandFingerprint;
  VendorCommand? _command;

  @override
  void initState() {
    super.initState();
    _load();
  }

  @override
  void dispose() {
    _account.dispose();
    _code.dispose();
    super.dispose();
  }

  Future<void> _load() async {
    setState(() {
      _loading = true;
      _loadError = null;
    });
    try {
      final r = await widget.runtime.backend.onboardingApi
          .getOnboardingBankProfile();
      _profile = r.data == null ? {} : r.object;
    } on ApiException catch (e) {
      if (e.statusCode == 404) {
        _profile = {};
      } else {
        _loadError = FinanceWorkspaceController.message(e);
      }
    } catch (e) {
      _loadError = FinanceWorkspaceController.message(e);
    }
    if (mounted) setState(() => _loading = false);
  }

  bool get _detailsValid =>
      _bank != null && RegExp(r'^\d{10}$').hasMatch(_account.text.trim());

  Future<void> _pickBank() async {
    final picked = await showModalBottomSheet<NigerianBank>(
      context: context,
      isScrollControlled: true,
      showDragHandle: true,
      builder: (context) => _BankPicker(selected: _bank),
    );
    if (picked != null && mounted) {
      setState(() {
        _bank = picked;
        _challenge = null;
        _error = null;
      });
    }
  }

  Future<void> _requestCode() async {
    if (!_detailsValid || _busy) return;
    FocusScope.of(context).unfocus();
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      final challenge = await widget.runtime.auth.startStepUp(
        'finance.bank_change',
      );
      _challenge = challenge;
      _challengeExpires = DateTime.now().add(
        Duration(seconds: challenge.expiresIn),
      );
      _code.text = challenge.debugOtp ?? '';
    } catch (e) {
      _error = FinanceWorkspaceController.message(e);
    }
    if (mounted) setState(() => _busy = false);
  }

  Future<void> _save() async {
    if (_busy || _challenge == null) return;
    if (_challengeExpires == null ||
        DateTime.now().isAfter(_challengeExpires!)) {
      setState(() {
        _challenge = null;
        _code.clear();
        _error = 'Your security code expired. Request a new code.';
      });
      return;
    }
    if (!RegExp(r'^\d{6}$').hasMatch(_code.text.trim())) {
      setState(() => _error = 'Enter the six-digit security code.');
      return;
    }
    FocusScope.of(context).unfocus();
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      final grant = await widget.runtime.auth.verifyStepUp(
        purpose: 'finance.bank_change',
        challengeId: _challenge!.challengeId,
        otp: _code.text.trim(),
      );
      _challenge = null;
      _code.clear();
      final version = (_profile['version'] as num?)?.toInt() ?? 0;
      final fingerprint = '${_bank!.code}:${_account.text.trim()}:$version';
      if (_commandFingerprint != fingerprint) {
        _commandFingerprint = fingerprint;
        _command = VendorCommand.create();
      }
      final r = await widget.runtime.backend.onboardingApi
          .putOnboardingBankProfile(
            command: _command,
            body: PutOnboardingBankProfileRequest(
              bankCode: _bank!.code,
              accountNumber: _account.text.trim(),
              stepUpGrantId: grant.grantId,
              expectedVersion: version,
            ),
          );
      _profile = r.object;
      _commandFingerprint = null;
      if (!mounted) return;
      setState(() => _busy = false);
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(
          content: Text(
            'Payout account saved: ${_profile['resolved_account_name'] ?? 'account confirmed'}',
          ),
        ),
      );
      Navigator.pop(context, true);
      return;
    } on ApiException catch (e) {
      _error = e.statusCode == 409
          ? 'Your bank details changed on another device. Pull to refresh and try again.'
          : FinanceWorkspaceController.message(e);
      if (e.statusCode == 409) await _load();
    } catch (e) {
      _error = FinanceWorkspaceController.message(e);
    }
    if (mounted) setState(() => _busy = false);
  }

  @override
  Widget build(BuildContext context) => Scaffold(
    appBar: AppBar(title: const Text('Payout account')),
    body: SafeArea(
      child: _loading
          ? const Center(child: CircularProgressIndicator())
          : RefreshIndicator(
              onRefresh: _load,
              child: ListView(
                padding: const EdgeInsets.fromLTRB(24, 8, 24, 32),
                physics: const AlwaysScrollableScrollPhysics(),
                children: [
                  const FinanceTitle(
                    'Where should we pay you?',
                    'Withdrawals go to this business bank account.',
                  ),
                  if (_loadError != null) ...[
                    FinanceNotice(_loadError!, error: true),
                    FinanceButton(
                      'Try again',
                      icon: Icons.refresh_rounded,
                      secondary: true,
                      onPressed: _load,
                    ),
                    const SizedBox(height: 16),
                  ],
                  if (_profile.isNotEmpty) ...[
                    FinanceCard(
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          Row(
                            children: [
                              const Icon(Icons.account_balance_outlined),
                              const Spacer(),
                              FinanceStatus(_profile['status']),
                            ],
                          ),
                          const SizedBox(height: 12),
                          Text(
                            '${_profile['resolved_account_name'] ?? ''}',
                            style: const TextStyle(
                              fontWeight: FontWeight.w700,
                              fontSize: 18,
                            ),
                          ),
                          const SizedBox(height: 4),
                          Text(
                            '${bankNameForCode(_profile['bank_code'])} - ${_profile['masked_account'] ?? ''}',
                          ),
                        ],
                      ),
                    ),
                    const SizedBox(height: 18),
                    const Text(
                      'Change account',
                      style: TextStyle(fontWeight: FontWeight.w700),
                    ),
                    const SizedBox(height: 10),
                  ],
                  InkWell(
                    borderRadius: BorderRadius.circular(14),
                    onTap: _busy ? null : _pickBank,
                    child: InputDecorator(
                      decoration: const InputDecoration(
                        labelText: 'Bank',
                        suffixIcon: Icon(Icons.expand_more_rounded),
                      ),
                      child: Text(_bank?.name ?? 'Choose your bank'),
                    ),
                  ),
                  const SizedBox(height: 14),
                  TextField(
                    controller: _account,
                    enabled: !_busy,
                    keyboardType: TextInputType.number,
                    inputFormatters: [
                      FilteringTextInputFormatter.digitsOnly,
                      LengthLimitingTextInputFormatter(10),
                    ],
                    decoration: const InputDecoration(
                      labelText: 'Account number',
                      helperText: 'Your 10-digit NUBAN account number',
                    ),
                    onChanged: (_) => setState(() => _challenge = null),
                  ),
                  const SizedBox(height: 18),
                  if (_challenge != null) ...[
                    const FinanceNotice(
                      'For your safety, enter the security code we sent to your sign-in contact.',
                    ),
                    TextField(
                      controller: _code,
                      enabled: !_busy,
                      keyboardType: TextInputType.number,
                      inputFormatters: [
                        FilteringTextInputFormatter.digitsOnly,
                        LengthLimitingTextInputFormatter(6),
                      ],
                      decoration: const InputDecoration(
                        labelText: 'Security code',
                      ),
                    ),
                    TextButton(
                      onPressed: _busy ? null : _requestCode,
                      child: const Text('Send a new code'),
                    ),
                    const SizedBox(height: 8),
                  ],
                  if (_error != null) FinanceNotice(_error!, error: true),
                  const SizedBox(height: 8),
                  FinanceButton(
                    _challenge == null
                        ? 'Continue'
                        : 'Confirm and save account',
                    icon: _challenge == null
                        ? Icons.arrow_forward_rounded
                        : Icons.verified_user_outlined,
                    busy: _busy,
                    onPressed: !_detailsValid || _busy
                        ? null
                        : (_challenge == null ? _requestCode : _save),
                  ),
                  const SizedBox(height: 12),
                  const Text(
                    'We check the account name with your bank before saving. Use an account in your business name.',
                  ),
                ],
              ),
            ),
    ),
  );
}

class _BankPicker extends StatefulWidget {
  const _BankPicker({this.selected});
  final NigerianBank? selected;
  @override
  State<_BankPicker> createState() => _BankPickerState();
}

class _BankPickerState extends State<_BankPicker> {
  String _query = '';
  @override
  Widget build(BuildContext context) {
    final banks = nigerianBanks
        .where((b) => b.name.toLowerCase().contains(_query.toLowerCase()))
        .toList();
    return SizedBox(
      height: MediaQuery.of(context).size.height * 0.75,
      child: Column(
        children: [
          Padding(
            padding: const EdgeInsets.fromLTRB(16, 0, 16, 8),
            child: TextField(
              autofocus: true,
              decoration: const InputDecoration(
                hintText: 'Search banks',
                prefixIcon: Icon(Icons.search_rounded),
              ),
              onChanged: (v) => setState(() => _query = v),
            ),
          ),
          Expanded(
            child: banks.isEmpty
                ? const Center(child: Text('No bank matches your search.'))
                : ListView.builder(
                    itemCount: banks.length,
                    itemBuilder: (context, i) => ListTile(
                      title: Text(banks[i].name),
                      trailing: widget.selected?.code == banks[i].code
                          ? const Icon(Icons.check_rounded)
                          : null,
                      onTap: () => Navigator.pop(context, banks[i]),
                    ),
                  ),
          ),
        ],
      ),
    );
  }
}
