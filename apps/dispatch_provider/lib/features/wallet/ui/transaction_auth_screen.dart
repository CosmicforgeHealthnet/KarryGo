import 'dart:math';

import 'package:flutter/material.dart';
import '../../../shared/widgets/numeric_keypad.dart';
import '../state/wallet_controller.dart';
import 'payment_processing_screen.dart';

class TransactionAuthScreen extends StatefulWidget {
  const TransactionAuthScreen({
    super.key,
    required this.walletController,
    required this.amount,
    required this.bankAccountId,
  });

  final WalletController walletController;
  final double amount;
  final String bankAccountId;

  @override
  State<TransactionAuthScreen> createState() => _TransactionAuthScreenState();
}

class _TransactionAuthScreenState extends State<TransactionAuthScreen> {
  static const int _pinLength = 4;

  bool _usePin = true;
  String _pin = '';
  final _passwordController = TextEditingController();
  bool _obscurePassword = true;
  bool _submitting = false;
  String? _error;

  // Stable idempotency key for this attempt — generated once on first submit.
  String? _idempotencyKey;

  bool get _isValid =>
      _usePin ? _pin.length == _pinLength : _passwordController.text.isNotEmpty;

  void _onKeyTap(String key) {
    setState(() {
      if (key == 'backspace') {
        if (_pin.isNotEmpty) _pin = _pin.substring(0, _pin.length - 1);
      } else if (_pin.length < _pinLength) {
        _pin += key;
      }
    });
  }

  String _generateIdempotencyKey() {
    final rand = Random.secure();
    final bytes = List<int>.generate(16, (_) => rand.nextInt(256));
    bytes[6] = (bytes[6] & 0x0f) | 0x40;
    bytes[8] = (bytes[8] & 0x3f) | 0x80;
    final hex =
        bytes.map((b) => b.toRadixString(16).padLeft(2, '0')).join();
    return '${hex.substring(0, 8)}-${hex.substring(8, 12)}-'
        '${hex.substring(12, 16)}-${hex.substring(16, 20)}-'
        '${hex.substring(20, 32)}';
  }

  Future<void> _submit() async {
    if (!_isValid || _submitting) return;

    // Generate a stable idempotency key per attempt.
    _idempotencyKey ??= _generateIdempotencyKey();
    final idempotencyKey = _idempotencyKey!;
    final amountKobo = (widget.amount * 100).round();

    setState(() {
      _submitting = true;
      _error = null;
    });

    final result = await widget.walletController.requestWithdrawal(
      bankAccountId: widget.bankAccountId,
      amountKobo: amountKobo,
      idempotencyKey: idempotencyKey,
    );

    if (!mounted) return;

    result.when(
      success: (_) {
        Navigator.of(context).pushReplacement(
          MaterialPageRoute(
            builder: (_) => PaymentProcessingScreen(amount: widget.amount),
          ),
        );
      },
      failure: (err) {
        setState(() {
          _submitting = false;
          _error = err.message.isNotEmpty
              ? err.message
              : 'Withdrawal failed. Please try again.';
          // Reset idempotency key on non-idempotent errors (anything except
          // network/timeout where retrying with the same key is correct).
          if (err.code != 'network') _idempotencyKey = null;
        });
      },
    );
  }

  @override
  void dispose() {
    _passwordController.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: Colors.white,
      body: SafeArea(
        child: Padding(
          padding: const EdgeInsets.fromLTRB(20, 12, 20, 20),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Row(
                children: [
                  GestureDetector(
                    onTap: () => Navigator.of(context).pop(),
                    behavior: HitTestBehavior.opaque,
                    child: const Padding(
                      padding: EdgeInsets.all(12),
                      child: Icon(
                        Icons.arrow_back_ios_new,
                        size: 18,
                        color: Color(0xFF1A1A1A),
                      ),
                    ),
                  ),
                  const SizedBox(width: 4),
                  const Expanded(
                    child: Text(
                      'Transaction Authorization',
                      style: TextStyle(
                        fontSize: 17,
                        fontWeight: FontWeight.w700,
                        color: Color(0xFF1A1A1A),
                      ),
                    ),
                  ),
                ],
              ),
              const SizedBox(height: 24),
              Text(
                _usePin
                    ? 'Please input your secure pin to authorize withdrawal'
                    : 'Please input your account password to authorize withdrawal',
                style: const TextStyle(fontSize: 13, color: Color(0xFF888888)),
              ),
              const SizedBox(height: 24),
              if (_usePin)
                ..._buildPinSection()
              else
                ..._buildPasswordSection(),
              if (_error != null) ...[
                const SizedBox(height: 12),
                Text(
                  _error!,
                  style: const TextStyle(
                    fontSize: 13,
                    color: Color(0xFFE53935),
                  ),
                  textAlign: TextAlign.center,
                ),
              ],
              const Spacer(),
              if (_usePin) NumericKeypad(onKeyTap: _onKeyTap),
              const SizedBox(height: 16),
              GestureDetector(
                onTap: (_isValid && !_submitting) ? _submit : null,
                child: Container(
                  width: double.infinity,
                  height: 52,
                  alignment: Alignment.center,
                  decoration: BoxDecoration(
                    color: (_isValid && !_submitting)
                        ? const Color(0xFF4CAF50)
                        : const Color(0xFFA8D5B5),
                    borderRadius: BorderRadius.circular(999),
                  ),
                  child: _submitting
                      ? const SizedBox(
                          width: 22,
                          height: 22,
                          child: CircularProgressIndicator(
                            color: Colors.white,
                            strokeWidth: 2,
                          ),
                        )
                      : Text(
                          _usePin ? 'Withdraw Now' : 'Confirm',
                          style: const TextStyle(
                            fontSize: 15,
                            fontWeight: FontWeight.w700,
                            color: Colors.white,
                          ),
                        ),
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }

  List<Widget> _buildPinSection() {
    return [
      Row(
        mainAxisAlignment: MainAxisAlignment.center,
        children: List.generate(_pinLength, (i) {
          final filled = i < _pin.length;
          return Container(
            width: 56,
            height: 56,
            margin: const EdgeInsets.symmetric(horizontal: 8),
            alignment: Alignment.center,
            decoration: BoxDecoration(
              shape: BoxShape.circle,
              border: Border.all(color: const Color(0xFF4CAF50), width: 1.5),
            ),
            child: Text(
              filled ? _pin[i] : '',
              style: const TextStyle(
                fontSize: 20,
                fontWeight: FontWeight.w700,
                color: Color(0xFF1A1A1A),
              ),
            ),
          );
        }),
      ),
      const SizedBox(height: 16),
      Center(
        child: GestureDetector(
          onTap: () => setState(() => _usePin = false),
          child: Text(
            'Use Password',
            style: TextStyle(
              fontSize: 13,
              fontWeight: FontWeight.w600,
              color: Colors.green.shade600,
            ),
          ),
        ),
      ),
    ];
  }

  List<Widget> _buildPasswordSection() {
    return [
      const Text(
        'Confirm Password',
        style: TextStyle(
          fontSize: 13,
          fontWeight: FontWeight.w700,
          color: Color(0xFF1A1A1A),
        ),
      ),
      const SizedBox(height: 8),
      Container(
        decoration: BoxDecoration(
          color: const Color(0xFFF5F5F5),
          borderRadius: BorderRadius.circular(12),
        ),
        child: TextField(
          controller: _passwordController,
          obscureText: _obscurePassword,
          onChanged: (_) => setState(() {}),
          decoration: InputDecoration(
            hintText: 'Confirm Password',
            hintStyle: const TextStyle(color: Color(0xFFAAAAAA)),
            border: InputBorder.none,
            contentPadding: const EdgeInsets.symmetric(
              horizontal: 16,
              vertical: 16,
            ),
            suffixIcon: IconButton(
              icon: Icon(
                _obscurePassword
                    ? Icons.visibility_off_outlined
                    : Icons.visibility_outlined,
                color: const Color(0xFFAAAAAA),
              ),
              onPressed: () =>
                  setState(() => _obscurePassword = !_obscurePassword),
            ),
          ),
        ),
      ),
      const SizedBox(height: 16),
      Center(
        child: GestureDetector(
          onTap: () => setState(() => _usePin = true),
          child: Text(
            'Use Pin',
            style: TextStyle(
              fontSize: 13,
              fontWeight: FontWeight.w600,
              color: Colors.green.shade600,
            ),
          ),
        ),
      ),
    ];
  }
}
