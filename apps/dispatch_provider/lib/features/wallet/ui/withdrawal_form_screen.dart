import 'package:flutter/material.dart';
import '../../../shared/widgets/numeric_keypad.dart';
import '../state/wallet_controller.dart';
import 'withdrawal_confirm_screen.dart';

class WithdrawalFormScreen extends StatefulWidget {
  const WithdrawalFormScreen({super.key, required this.walletController});

  final WalletController walletController;

  @override
  State<WithdrawalFormScreen> createState() => _WithdrawalFormScreenState();
}

class _WithdrawalFormScreenState extends State<WithdrawalFormScreen> {
  // how much of the card hangs below the green header
  static const double _cardOverhang = 36.0;
  // card height
  static const double _cardHeight = 88.0;

  String _amount = '';

  WalletController get _controller => widget.walletController;

  double get _availableBalance =>
      (_controller.earnings?.availableKobo ?? 0) / 100;

  bool get _isValid {
    final value = double.tryParse(_amount) ?? 0;
    return value > 0 && value <= _availableBalance;
  }

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

  static String _formatAmount(double value) {
    final s = value.toStringAsFixed(2);
    final parts = s.split('.');
    final whole = parts[0];
    final buffer = StringBuffer();
    for (int i = 0; i < whole.length; i++) {
      if (i != 0 && (whole.length - i) % 3 == 0) buffer.write(',');
      buffer.write(whole[i]);
    }
    return '₦ $buffer.${parts[1]}';
  }

  @override
  Widget build(BuildContext context) {
    final topPadding = MediaQuery.of(context).padding.top;

    return Scaffold(
      backgroundColor: Colors.white,
      body: Column(
        children: [
          // ── header + overlapping balance card ─────────────────────────
          Stack(
            clipBehavior: Clip.none,
            children: [
              // green header
              Container(
                width: double.infinity,
                padding: EdgeInsets.fromLTRB(
                  16,
                  topPadding + 12,
                  16,
                  _cardHeight - _cardOverhang + 16,
                ),
                decoration: const BoxDecoration(
                  color: Color(0xFFB7E4C7),
                  borderRadius: BorderRadius.vertical(
                    bottom: Radius.circular(28),
                  ),
                ),
                child: Row(
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
                    const SizedBox(width: 12),
                    const Text(
                      'Withdrawal Form',
                      style: TextStyle(
                        fontSize: 18,
                        fontWeight: FontWeight.w700,
                        color: Color(0xFF1A1A1A),
                      ),
                    ),
                  ],
                ),
              ),
              // balance card — straddles the bottom edge
              Positioned(
                bottom: -_cardOverhang,
                left: 16,
                right: 16,
                child: Container(
                  height: _cardHeight,
                  padding: const EdgeInsets.fromLTRB(18, 14, 18, 14),
                  decoration: BoxDecoration(
                    gradient: const LinearGradient(
                      colors: [Color(0xFF1B5E20), Color(0xFF388E3C)],
                      begin: Alignment.topLeft,
                      end: Alignment.bottomRight,
                    ),
                    borderRadius: BorderRadius.circular(16),
                  ),
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    mainAxisAlignment: MainAxisAlignment.center,
                    children: [
                      const Text(
                        'Available Balance',
                        style: TextStyle(
                          fontSize: 13,
                          color: Colors.white70,
                          fontWeight: FontWeight.w500,
                        ),
                      ),
                      const SizedBox(height: 4),
                      ListenableBuilder(
                        listenable: _controller,
                        builder: (_, _) => Text(
                          _controller.isLoading
                              ? '₦ ---'
                              : _formatAmount(_availableBalance),
                          style: const TextStyle(
                            fontSize: 22,
                            fontWeight: FontWeight.w800,
                            color: Colors.white,
                            letterSpacing: 0.3,
                          ),
                        ),
                      ),
                    ],
                  ),
                ),
              ),
            ],
          ),

          SizedBox(height: _cardOverhang + 20),

          // ── amount input ──────────────────────────────────────────────
          Padding(
            padding: const EdgeInsets.symmetric(horizontal: 20),
            child: Container(
              width: double.infinity,
              padding: const EdgeInsets.symmetric(horizontal: 20, vertical: 16),
              decoration: BoxDecoration(
                color: const Color(0xFFF2F2F2),
                borderRadius: BorderRadius.circular(14),
              ),
              child: Row(
                children: [
                  Text(
                    '₦',
                    style: TextStyle(
                      fontSize: 22,
                      fontWeight: FontWeight.w700,
                      color: Colors.green.shade200,
                    ),
                  ),
                  const SizedBox(width: 10),
                  Text(
                    _amount.isEmpty ? '0' : _amount,
                    style: const TextStyle(
                      fontSize: 22,
                      fontWeight: FontWeight.w800,
                      color: Color(0xFF4CAF50),
                    ),
                  ),
                ],
              ),
            ),
          ),

          const Spacer(),

          NumericKeypad(onKeyTap: _onKeyTap),
          const SizedBox(height: 16),

          SafeArea(
            top: false,
            child: Padding(
              padding: const EdgeInsets.fromLTRB(20, 0, 20, 20),
              child: GestureDetector(
                onTap: _isValid
                    ? () => Navigator.of(context).push(
                        MaterialPageRoute(
                          builder: (_) => WithdrawalConfirmScreen(
                            walletController: _controller,
                            amount: double.parse(_amount),
                          ),
                        ),
                      )
                    : null,
                child: Container(
                  width: double.infinity,
                  height: 52,
                  alignment: Alignment.center,
                  decoration: BoxDecoration(
                    color: _isValid
                        ? const Color(0xFF4CAF50)
                        : const Color(0xFFA8D5B5),
                    borderRadius: BorderRadius.circular(999),
                  ),
                  child: const Text(
                    'Withdraw Now',
                    style: TextStyle(
                      fontSize: 15,
                      fontWeight: FontWeight.w700,
                      color: Colors.white,
                    ),
                  ),
                ),
              ),
            ),
          ),
        ],
      ),
    );
  }
}
