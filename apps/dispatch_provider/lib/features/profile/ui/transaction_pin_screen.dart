import '../../../core/theme/theme_extensions.dart';
import 'package:flutter/material.dart';
import '../../../../shared/widgets/numeric_keypad.dart';
import '../../wallet/state/wallet_controller.dart';
import 'create_pin_screen.dart';

class TransactionPinScreen extends StatefulWidget {
  const TransactionPinScreen({super.key, required this.walletController});

  final WalletController walletController;

  @override
  State<TransactionPinScreen> createState() => _TransactionPinScreenState();
}

class _TransactionPinScreenState extends State<TransactionPinScreen> {
  bool _hasPin = false;
  bool _enabled = false;
  String _pin = '';

  @override
  void initState() {
    super.initState();
    widget.walletController.loadPinStatus().then((result) {
      if (!mounted) return;
      result.when(
        success: (isSet) => setState(() {
          _hasPin = isSet;
          _enabled = isSet;
        }),
        failure: (_) {},
      );
    });
  }

  void _onToggle(bool value) {
    setState(() {
      _enabled = value;
      _pin = '';
    });
    // Turning the pin on without one set goes straight to creation.
    if (value && !_hasPin) _goToCreatePin();
  }

  void _onKey(String key) {
    if (!_enabled) return;
    setState(() {
      if (key == 'backspace') {
        if (_pin.isNotEmpty) _pin = _pin.substring(0, _pin.length - 1);
      } else if (_pin.length < 4) {
        _pin += key;
      }
    });

    if (_pin.length == 4) {
      if (_hasPin) {
        // The typed digits are the CURRENT pin, verified server-side when the
        // change is submitted from the create screen.
        _showChangeConfirmation(currentPin: _pin);
      } else {
        _goToCreatePin();
      }
    }
  }

  Future<void> _goToCreatePin() async {
    setState(() => _pin = '');
    final created = await Navigator.of(context).push<bool>(
      MaterialPageRoute(
        builder: (_) =>
            CreatePinScreen(walletController: widget.walletController),
      ),
    );
    if (!mounted) return;
    if (created == true) {
      setState(() {
        _hasPin = true;
        _enabled = true;
      });
    }
  }

  void _showChangeConfirmation({required String currentPin}) {
    showDialog<bool>(
      context: context,
      barrierDismissible: false,
      builder: (_) => const _ChangePinDialog(),
    ).then((confirmed) async {
      if (!mounted) return;
      setState(() => _pin = '');
      if (confirmed == true) {
        await Navigator.of(context).push<bool>(
          MaterialPageRoute(
            builder: (_) => CreatePinScreen(
              walletController: widget.walletController,
              currentPin: currentPin,
            ),
          ),
        );
      }
    });
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: context.palette.surface,
      body: SafeArea(
        child: Column(
          children: [
            Padding(
              padding: const EdgeInsets.fromLTRB(20, 12, 20, 0),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
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
                    'Transaction Pin',
                    style: TextStyle(
                      fontSize: 20,
                      fontWeight: FontWeight.w700,
                      color: Color(0xFF1A1A1A),
                    ),
                  ),
                  const SizedBox(height: 2),
                  const Text(
                    'Manage Transaction Pin and access to your earnings.',
                    style: TextStyle(fontSize: 13, color: Color(0xFF888888)),
                  ),
                  const SizedBox(height: 24),
                  Container(
                    padding: const EdgeInsets.symmetric(
                      horizontal: 16,
                      vertical: 14,
                    ),
                    decoration: BoxDecoration(
                      border: Border.all(color: const Color(0xFFEEEEEE)),
                      borderRadius: BorderRadius.circular(12),
                    ),
                    child: Row(
                      children: [
                        Expanded(
                          child: Column(
                            crossAxisAlignment: CrossAxisAlignment.start,
                            children: const [
                              Text(
                                'Transaction Pin',
                                style: TextStyle(
                                  fontSize: 14,
                                  fontWeight: FontWeight.w700,
                                  color: Color(0xFF1A1A1A),
                                ),
                              ),
                              SizedBox(height: 2),
                              Text(
                                'Do you want to add transaction pin to your account.',
                                style: TextStyle(
                                  fontSize: 12,
                                  color: Color(0xFF888888),
                                ),
                              ),
                            ],
                          ),
                        ),
                        Switch(
                          value: _enabled,
                          onChanged: _onToggle,
                          activeColor: const Color(0xFF05124D),
                        ),
                      ],
                    ),
                  ),
                ],
              ),
            ),
            if (_enabled) ...[
              const SizedBox(height: 32),
              Text(
                'Enter Pin!',
                style: const TextStyle(
                  fontSize: 18,
                  fontWeight: FontWeight.w700,
                  color: Color(0xFF1A1A1A),
                ),
              ),
              const SizedBox(height: 24),
              _PinDots(pin: _pin, hasError: false),
              const SizedBox(height: 32),
              NumericKeypad(onKeyTap: _onKey),
              const Spacer(),
              GestureDetector(
                onTap: _hasPin && _pin.length == 4
                    ? () => _showChangeConfirmation(currentPin: _pin)
                    : null,
                child: Padding(
                  padding: const EdgeInsets.only(bottom: 16),
                  child: Text(
                    'Change Pin?',
                    style: TextStyle(
                      fontSize: 14,
                      fontWeight: FontWeight.w700,
                      color: _hasPin
                          ? const Color(0xFF05124D)
                          : const Color(0xFFAAAAAA),
                    ),
                  ),
                ),
              ),
            ] else
              const Spacer(),
          ],
        ),
      ),
    );
  }
}

class _PinDots extends StatelessWidget {
  const _PinDots({required this.pin, required this.hasError});
  final String pin;
  final bool hasError;

  @override
  Widget build(BuildContext context) {
    return Row(
      mainAxisAlignment: MainAxisAlignment.center,
      children: List.generate(4, (i) {
        final filled = i < pin.length;
        final color = hasError
            ? const Color(0xFFE53935)
            : filled
            ? const Color(0xFF05124D)
            : const Color(0xFFE53935);

        return Container(
          margin: const EdgeInsets.symmetric(horizontal: 10),
          width: 52,
          height: 52,
          alignment: Alignment.center,
          decoration: BoxDecoration(
            shape: BoxShape.circle,
            border: Border.all(color: color, width: 1.5),
          ),
          child: Text(
            filled ? pin[i] : '0',
            style: TextStyle(
              fontSize: 20,
              fontWeight: FontWeight.w700,
              color: color,
            ),
          ),
        );
      }),
    );
  }
}

class _ChangePinDialog extends StatelessWidget {
  const _ChangePinDialog();

  @override
  Widget build(BuildContext context) {
    return Dialog(
      shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(20)),
      child: Padding(
        padding: const EdgeInsets.fromLTRB(24, 28, 24, 24),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Image.asset(
              'assets/figma/profile_submitted.png',
              height: 120,
              errorBuilder: (_, __, ___) => const SizedBox(height: 120),
            ),
            const SizedBox(height: 20),
            const Text(
              'Change Transaction Pin?',
              style: TextStyle(
                fontSize: 18,
                fontWeight: FontWeight.w700,
                color: Color(0xFF1A1A1A),
              ),
              textAlign: TextAlign.center,
            ),
            const SizedBox(height: 8),
            const Text(
              'This action could affect your access to funds. Are you sure you want to change transaction pin?',
              style: TextStyle(
                fontSize: 13,
                color: Color(0xFF888888),
                height: 1.5,
              ),
              textAlign: TextAlign.center,
            ),
            const SizedBox(height: 24),
            SizedBox(
              width: double.infinity,
              height: 52,
              child: FilledButton(
                onPressed: () => Navigator.of(context).pop(true),
                style: FilledButton.styleFrom(
                  backgroundColor: const Color(0xFF05124D),
                  shape: RoundedRectangleBorder(
                    borderRadius: BorderRadius.circular(999),
                  ),
                ),
                child: const Text(
                  'Yes',
                  style: TextStyle(
                    fontSize: 16,
                    fontWeight: FontWeight.w700,
                    color: Color(0xFF081F44),
                  ),
                ),
              ),
            ),
            const SizedBox(height: 10),
            SizedBox(
              width: double.infinity,
              height: 52,
              child: FilledButton(
                onPressed: () => Navigator.of(context).pop(false),
                style: FilledButton.styleFrom(
                  backgroundColor: const Color(
                    0xFF05124D,
                  ).withValues(alpha: 0.15),
                  shape: RoundedRectangleBorder(
                    borderRadius: BorderRadius.circular(999),
                  ),
                ),
                child: const Text(
                  'No',
                  style: TextStyle(
                    fontSize: 16,
                    fontWeight: FontWeight.w700,
                    color: Color(0xFF05124D),
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
