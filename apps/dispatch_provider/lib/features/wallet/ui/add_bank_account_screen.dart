import 'package:flutter/material.dart';

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
  final _bankCodeController = TextEditingController();

  BankAccount? _resolved;
  bool _resolving = false;
  bool _registering = false;
  String? _error;

  @override
  void dispose() {
    _accountNumberController.dispose();
    _bankCodeController.dispose();
    super.dispose();
  }

  bool get _canResolve =>
      _accountNumberController.text.trim().length >= 10 &&
      _bankCodeController.text.trim().isNotEmpty;

  Future<void> _onVerify() async {
    setState(() {
      _resolving = true;
      _error = null;
      _resolved = null;
    });
    final result = await widget.walletController.resolveBankAccount(
      accountNumber: _accountNumberController.text.trim(),
      bankCode: _bankCodeController.text.trim(),
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
            : 'Could not verify account. Check account number and bank code.';
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
    final result = await widget.walletController.registerBankAccount(
      bankCode: resolved.bankCode.isNotEmpty
          ? resolved.bankCode
          : _bankCodeController.text.trim(),
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
    return Scaffold(
      backgroundColor: Colors.white,
      appBar: AppBar(
        backgroundColor: Colors.white,
        elevation: 0,
        leading: GestureDetector(
          onTap: () => Navigator.of(context).pop(),
          behavior: HitTestBehavior.opaque,
          child: const Padding(
            padding: EdgeInsets.all(12),
            child: Icon(Icons.arrow_back_ios_new, size: 18, color: Color(0xFF1A1A1A)),
          ),
        ),
        title: const Text(
          'Add Bank Account',
          style: TextStyle(fontSize: 16, fontWeight: FontWeight.w700, color: Color(0xFF1A1A1A)),
        ),
        centerTitle: false,
      ),
      body: SingleChildScrollView(
        padding: const EdgeInsets.all(20),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            const Text(
              'Bank Code',
              style: TextStyle(fontSize: 13, fontWeight: FontWeight.w600, color: Color(0xFF1A1A1A)),
            ),
            const SizedBox(height: 6),
            Container(
              decoration: BoxDecoration(
                color: const Color(0xFFF5F5F5),
                borderRadius: BorderRadius.circular(12),
              ),
              child: TextField(
                controller: _bankCodeController,
                keyboardType: TextInputType.number,
                onChanged: (_) => setState(() { _resolved = null; _error = null; }),
                decoration: const InputDecoration(
                  hintText: 'e.g. 044',
                  hintStyle: TextStyle(color: Color(0xFFAAAAAA)),
                  border: InputBorder.none,
                  contentPadding: EdgeInsets.symmetric(horizontal: 16, vertical: 14),
                ),
              ),
            ),
            const SizedBox(height: 16),
            const Text(
              'Account Number',
              style: TextStyle(fontSize: 13, fontWeight: FontWeight.w600, color: Color(0xFF1A1A1A)),
            ),
            const SizedBox(height: 6),
            Container(
              decoration: BoxDecoration(
                color: const Color(0xFFF5F5F5),
                borderRadius: BorderRadius.circular(12),
              ),
              child: TextField(
                controller: _accountNumberController,
                keyboardType: TextInputType.number,
                maxLength: 10,
                onChanged: (_) => setState(() { _resolved = null; _error = null; }),
                decoration: const InputDecoration(
                  hintText: '10-digit account number',
                  hintStyle: TextStyle(color: Color(0xFFAAAAAA)),
                  border: InputBorder.none,
                  counterText: '',
                  contentPadding: EdgeInsets.symmetric(horizontal: 16, vertical: 14),
                ),
              ),
            ),
            const SizedBox(height: 20),
            SizedBox(
              width: double.infinity,
              height: 50,
              child: ElevatedButton(
                onPressed: (_canResolve && !_resolving && _resolved == null)
                    ? _onVerify
                    : null,
                style: ElevatedButton.styleFrom(
                  backgroundColor: const Color(0xFF4CAF50),
                  disabledBackgroundColor: const Color(0xFFA8D5B5),
                  shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(999)),
                ),
                child: _resolving
                    ? const SizedBox(
                        width: 20,
                        height: 20,
                        child: CircularProgressIndicator(color: Colors.white, strokeWidth: 2),
                      )
                    : const Text(
                        'Verify Account',
                        style: TextStyle(fontSize: 15, fontWeight: FontWeight.w700, color: Colors.white),
                      ),
              ),
            ),
            if (_error != null) ...[
              const SizedBox(height: 12),
              Text(
                _error!,
                style: const TextStyle(fontSize: 13, color: Color(0xFFE53935)),
              ),
            ],
            if (_resolved != null) ...[
              const SizedBox(height: 20),
              Container(
                width: double.infinity,
                padding: const EdgeInsets.all(16),
                decoration: BoxDecoration(
                  color: const Color(0xFFF0FFF0),
                  borderRadius: BorderRadius.circular(12),
                  border: Border.all(color: const Color(0xFF4CAF50), width: 1),
                ),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      _resolved!.accountName,
                      style: const TextStyle(fontSize: 15, fontWeight: FontWeight.w700, color: Color(0xFF1A1A1A)),
                    ),
                    const SizedBox(height: 4),
                    Text(
                      _resolved!.bankName,
                      style: const TextStyle(fontSize: 13, color: Color(0xFF888888)),
                    ),
                    Text(
                      _resolved!.accountNumber,
                      style: const TextStyle(fontSize: 13, color: Color(0xFF888888)),
                    ),
                  ],
                ),
              ),
              const SizedBox(height: 16),
              SizedBox(
                width: double.infinity,
                height: 50,
                child: ElevatedButton(
                  onPressed: _registering ? null : _onRegister,
                  style: ElevatedButton.styleFrom(
                    backgroundColor: const Color(0xFF4CAF50),
                    disabledBackgroundColor: const Color(0xFFA8D5B5),
                    shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(999)),
                  ),
                  child: _registering
                      ? const SizedBox(
                          width: 20,
                          height: 20,
                          child: CircularProgressIndicator(color: Colors.white, strokeWidth: 2),
                        )
                      : const Text(
                          'Use This Account',
                          style: TextStyle(fontSize: 15, fontWeight: FontWeight.w700, color: Colors.white),
                        ),
                ),
              ),
            ],
          ],
        ),
      ),
    );
  }
}
