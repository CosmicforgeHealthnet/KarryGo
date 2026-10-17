import '../../../core/theme/theme_extensions.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

class PaymentsWithdrawalsScreen extends StatefulWidget {
  const PaymentsWithdrawalsScreen({super.key});

  @override
  State<PaymentsWithdrawalsScreen> createState() =>
      _PaymentsWithdrawalsScreenState();
}

class _PaymentsWithdrawalsScreenState extends State<PaymentsWithdrawalsScreen> {
  final List<_BankAccount> _accounts = [];
  int _defaultIndex = 0;

  void _onAddAccount() {
    Navigator.of(context).push(
      MaterialPageRoute(
        builder: (_) => _AddBankScreen(
          onSave: (account) {
            setState(() => _accounts.add(account));
          },
        ),
      ),
    );
  }

  void _onDelete(int index) {
    setState(() {
      _accounts.removeAt(index);
      if (_defaultIndex >= _accounts.length && _accounts.isNotEmpty) {
        _defaultIndex = 0;
      }
    });
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: context.palette.surface,
      body: SafeArea(
        child: ListView(
          padding: const EdgeInsets.fromLTRB(20, 12, 20, 32),
          children: [
            GestureDetector(
              behavior: HitTestBehavior.opaque,
              onTap: () => Navigator.of(context).pop(),
              child: const Align(
                alignment: Alignment.centerLeft,
                child: Icon(
                  Icons.arrow_back_ios_new,
                  size: 20,
                  color: Color(0xFF1A1A1A),
                ),
              ),
            ),
            const SizedBox(height: 16),
            const Text(
              'Payments & Withdrawals',
              style: TextStyle(
                fontSize: 20,
                fontWeight: FontWeight.w700,
                color: Color(0xFF1A1A1A),
              ),
            ),
            const SizedBox(height: 2),
            const Text(
              'Manage your payment Information here.',
              style: TextStyle(fontSize: 13, color: Color(0xFF888888)),
            ),
            const SizedBox(height: 24),
            Row(
              mainAxisAlignment: MainAxisAlignment.spaceBetween,
              children: [
                const Text(
                  'Linked Accounts',
                  style: TextStyle(
                    fontSize: 16,
                    fontWeight: FontWeight.w700,
                    color: Color(0xFF1A1A1A),
                  ),
                ),
                GestureDetector(
                  onTap: _onAddAccount,
                  child: Container(
                    padding: const EdgeInsets.symmetric(
                      horizontal: 14,
                      vertical: 8,
                    ),
                    decoration: BoxDecoration(
                      color: const Color(0xFF05124D),
                      borderRadius: BorderRadius.circular(999),
                    ),
                    child: const Text(
                      'Add Bank Account',
                      style: TextStyle(
                        fontSize: 12,
                        fontWeight: FontWeight.w700,
                        color: Colors.white,
                      ),
                    ),
                  ),
                ),
              ],
            ),
            const SizedBox(height: 12),
            if (_accounts.isEmpty)
              const Padding(
                padding: EdgeInsets.symmetric(vertical: 16),
                child: Text(
                  'No linked accounts yet.',
                  style: TextStyle(fontSize: 13, color: Color(0xFF888888)),
                ),
              )
            else
              Container(
                decoration: BoxDecoration(
                  border: Border.all(color: const Color(0xFFEEEEEE)),
                  borderRadius: BorderRadius.circular(12),
                ),
                child: Column(
                  children: _accounts.asMap().entries.map((entry) {
                    final i = entry.key;
                    final acc = entry.value;
                    return Column(
                      children: [
                        Padding(
                          padding: const EdgeInsets.symmetric(
                            horizontal: 16,
                            vertical: 14,
                          ),
                          child: Row(
                            children: [
                              Expanded(child: _AccountInfo(account: acc)),
                              GestureDetector(
                                onTap: () => _onDelete(i),
                                child: const Icon(
                                  Icons.delete_outline,
                                  color: Color(0xFFE53935),
                                  size: 22,
                                ),
                              ),
                            ],
                          ),
                        ),
                        if (i < _accounts.length - 1)
                          const Divider(height: 1, color: Color(0xFFEEEEEE)),
                      ],
                    );
                  }).toList(),
                ),
              ),
            if (_accounts.isNotEmpty) ...[
              const SizedBox(height: 24),
              const Text(
                'Set Default withdrawal Account',
                style: TextStyle(
                  fontSize: 16,
                  fontWeight: FontWeight.w700,
                  color: Color(0xFF1A1A1A),
                ),
              ),
              const SizedBox(height: 12),
              Container(
                decoration: BoxDecoration(
                  border: Border.all(color: const Color(0xFFEEEEEE)),
                  borderRadius: BorderRadius.circular(12),
                ),
                child: Column(
                  children: _accounts.asMap().entries.map((entry) {
                    final i = entry.key;
                    final acc = entry.value;
                    final isDefault = _defaultIndex == i;
                    return Column(
                      children: [
                        GestureDetector(
                          behavior: HitTestBehavior.opaque,
                          onTap: () => setState(() => _defaultIndex = i),
                          child: Padding(
                            padding: const EdgeInsets.symmetric(
                              horizontal: 16,
                              vertical: 14,
                            ),
                            child: Row(
                              children: [
                                Expanded(child: _AccountInfo(account: acc)),
                                Container(
                                  width: 20,
                                  height: 20,
                                  decoration: BoxDecoration(
                                    shape: BoxShape.circle,
                                    border: Border.all(
                                      color: const Color(0xFF05124D),
                                      width: 1.5,
                                    ),
                                    color: isDefault
                                        ? const Color(0xFF05124D)
                                        : Colors.transparent,
                                  ),
                                  child: isDefault
                                      ? const Icon(
                                          Icons.circle,
                                          size: 10,
                                          color: Colors.white,
                                        )
                                      : null,
                                ),
                              ],
                            ),
                          ),
                        ),
                        if (i < _accounts.length - 1)
                          const Divider(height: 1, color: Color(0xFFEEEEEE)),
                      ],
                    );
                  }).toList(),
                ),
              ),
            ],
          ],
        ),
      ),
    );
  }
}

class _AccountInfo extends StatelessWidget {
  const _AccountInfo({required this.account});
  final _BankAccount account;

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(
          account.accountName,
          style: const TextStyle(
            fontSize: 13,
            fontWeight: FontWeight.w700,
            color: Color(0xFF1A1A1A),
          ),
        ),
        Text(
          account.bankName,
          style: const TextStyle(fontSize: 13, color: Color(0xFF1A1A1A)),
        ),
        Text(
          account.accountNumber,
          style: const TextStyle(fontSize: 13, color: Color(0xFF888888)),
        ),
      ],
    );
  }
}

class _BankAccount {
  final String bankName;
  final String accountNumber;
  final String accountName;

  const _BankAccount({
    required this.bankName,
    required this.accountNumber,
    required this.accountName,
  });
}

class _AddBankScreen extends StatefulWidget {
  const _AddBankScreen({required this.onSave});
  final ValueChanged<_BankAccount> onSave;

  @override
  State<_AddBankScreen> createState() => _AddBankScreenState();
}

class _AddBankScreenState extends State<_AddBankScreen> {
  String? _selectedBank;
  final _accountNumberController = TextEditingController();
  final _accountNameController = TextEditingController();

  static const _banks = [
    'Access Bank',
    'Fidelity Bank',
    'First Bank',
    'GT Bank',
    'Kuda Bank',
    'Opay',
    'Palmpay',
    'Polaris Bank',
    'Sterling Bank',
    'UBA',
    'Union Bank',
    'Wema Bank',
    'Zenith Bank',
  ];

  bool get _canSave =>
      _selectedBank != null &&
      _accountNumberController.text.trim().isNotEmpty &&
      _accountNameController.text.trim().isNotEmpty;

  @override
  void dispose() {
    _accountNumberController.dispose();
    _accountNameController.dispose();
    super.dispose();
  }

  void _onSave() {
    widget.onSave(
      _BankAccount(
        bankName: _selectedBank!,
        accountNumber: _accountNumberController.text.trim(),
        accountName: _accountNameController.text.trim(),
      ),
    );
    Navigator.of(context).pop();
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: context.palette.surface,
      body: SafeArea(
        child: Column(
          children: [
            Expanded(
              child: ListView(
                padding: const EdgeInsets.fromLTRB(20, 12, 20, 24),
                children: [
                  GestureDetector(
                    behavior: HitTestBehavior.opaque,
                    onTap: () => Navigator.of(context).pop(),
                    child: const Align(
                      alignment: Alignment.centerLeft,
                      child: Icon(
                        Icons.arrow_back_ios_new,
                        size: 20,
                        color: Color(0xFF1A1A1A),
                      ),
                    ),
                  ),
                  const SizedBox(height: 16),
                  const Text(
                    'Payments & Withdrawals',
                    style: TextStyle(
                      fontSize: 20,
                      fontWeight: FontWeight.w700,
                      color: Color(0xFF1A1A1A),
                    ),
                  ),
                  const SizedBox(height: 2),
                  const Text(
                    'Manage your payment Information here.',
                    style: TextStyle(fontSize: 13, color: Color(0xFF888888)),
                  ),
                  const SizedBox(height: 24),
                  const Text(
                    'Enter New Bank Details',
                    style: TextStyle(
                      fontSize: 18,
                      fontWeight: FontWeight.w700,
                      color: Color(0xFF1A1A1A),
                    ),
                  ),
                  const SizedBox(height: 20),
                  const _FieldLabel('Bank'),
                  const SizedBox(height: 8),
                  Container(
                    padding: const EdgeInsets.symmetric(horizontal: 16),
                    decoration: BoxDecoration(
                      color: const Color(0xFFF5F6F8),
                      borderRadius: BorderRadius.circular(12),
                    ),
                    child: DropdownButtonHideUnderline(
                      child: DropdownButton<String>(
                        value: _selectedBank,
                        isExpanded: true,
                        hint: const Text(
                          'Select Bank',
                          style: TextStyle(
                            color: Color(0xFFAAAAAA),
                            fontSize: 14,
                          ),
                        ),
                        icon: const Icon(
                          Icons.keyboard_arrow_down,
                          color: Color(0xFF888888),
                        ),
                        items: _banks
                            .map(
                              (b) => DropdownMenuItem(
                                value: b,
                                child: Text(
                                  b,
                                  style: const TextStyle(
                                    fontSize: 14,
                                    color: Color(0xFF1A1A1A),
                                  ),
                                ),
                              ),
                            )
                            .toList(),
                        onChanged: (v) => setState(() => _selectedBank = v),
                      ),
                    ),
                  ),
                  const SizedBox(height: 16),
                  const _FieldLabel('Account Number'),
                  const SizedBox(height: 8),
                  TextField(
                    controller: _accountNumberController,
                    keyboardType: TextInputType.number,
                    inputFormatters: [FilteringTextInputFormatter.digitsOnly],
                    onChanged: (_) => setState(() {}),
                    style: const TextStyle(
                      fontSize: 14,
                      color: Color(0xFF1A1A1A),
                    ),
                    decoration: InputDecoration(
                      hintText: 'Enter Acct No',
                      hintStyle: const TextStyle(
                        color: Color(0xFFAAAAAA),
                        fontSize: 14,
                      ),
                      filled: true,
                      fillColor: const Color(0xFFF5F6F8),
                      contentPadding: const EdgeInsets.symmetric(
                        horizontal: 16,
                        vertical: 14,
                      ),
                      border: OutlineInputBorder(
                        borderRadius: BorderRadius.circular(12),
                        borderSide: BorderSide.none,
                      ),
                    ),
                  ),
                  const SizedBox(height: 16),
                  const _FieldLabel('Account Name'),
                  const SizedBox(height: 8),
                  TextField(
                    controller: _accountNameController,
                    onChanged: (_) => setState(() {}),
                    style: const TextStyle(
                      fontSize: 14,
                      color: Color(0xFF1A1A1A),
                    ),
                    decoration: InputDecoration(
                      hintText: 'Enter Acct Name',
                      hintStyle: const TextStyle(
                        color: Color(0xFFAAAAAA),
                        fontSize: 14,
                      ),
                      filled: true,
                      fillColor: const Color(0xFFF5F6F8),
                      contentPadding: const EdgeInsets.symmetric(
                        horizontal: 16,
                        vertical: 14,
                      ),
                      border: OutlineInputBorder(
                        borderRadius: BorderRadius.circular(12),
                        borderSide: BorderSide.none,
                      ),
                    ),
                  ),
                ],
              ),
            ),
            Padding(
              padding: const EdgeInsets.fromLTRB(20, 0, 20, 16),
              child: SizedBox(
                height: 52,
                width: double.infinity,
                child: FilledButton(
                  onPressed: _canSave ? _onSave : null,
                  style: FilledButton.styleFrom(
                    backgroundColor: const Color(0xFF05124D),
                    disabledBackgroundColor: const Color(
                      0xFF05124D,
                    ).withValues(alpha: 0.35),
                    shape: RoundedRectangleBorder(
                      borderRadius: BorderRadius.circular(999),
                    ),
                  ),
                  child: const Text(
                    'Save',
                    style: TextStyle(
                      fontSize: 16,
                      fontWeight: FontWeight.w700,
                      color: Color(0xFF081F44),
                    ),
                  ),
                ),
              ),
            ),
          ],
        ),
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
      style: const TextStyle(
        fontSize: 13,
        fontWeight: FontWeight.w600,
        color: Color(0xFF1A1A1A),
      ),
    );
  }
}
