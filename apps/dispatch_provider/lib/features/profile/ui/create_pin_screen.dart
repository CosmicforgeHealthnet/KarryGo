import 'package:flutter/material.dart';
import '../../../../shared/widgets/numeric_keypad.dart';

class CreatePinScreen extends StatefulWidget {
  const CreatePinScreen({super.key, this.currentPin});
  final String? currentPin;

  @override
  State<CreatePinScreen> createState() => _CreatePinScreenState();
}

class _CreatePinScreenState extends State<CreatePinScreen> {
  String _newPin = '';
  String _confirmPin = '';
  bool _isConfirming = false;

  void _onKey(String key) {
    setState(() {
      if (_isConfirming) {
        if (key == 'backspace') {
          if (_confirmPin.isNotEmpty) {
            _confirmPin = _confirmPin.substring(0, _confirmPin.length - 1);
          }
        } else if (_confirmPin.length < 4) {
          _confirmPin += key;
        }
      } else {
        if (key == 'backspace') {
          if (_newPin.isNotEmpty) {
            _newPin = _newPin.substring(0, _newPin.length - 1);
          }
        } else if (_newPin.length < 4) {
          _newPin += key;
        }
      }
    });

    if (!_isConfirming && _newPin.length == 4) {
      setState(() => _isConfirming = true);
    }
  }

  bool get _pinsMatch => _confirmPin == _newPin && _confirmPin.length == 4;

  void _submit() {
    // TODO: wire to API
    Navigator.of(context).pop();
  }

  @override
  Widget build(BuildContext context) {
    final pin = _isConfirming ? _confirmPin : _newPin;

    return Scaffold(
      backgroundColor: Colors.white,
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
                    onTap: () {
                      if (_isConfirming) {
                        setState(() {
                          _isConfirming = false;
                          _confirmPin = '';
                        });
                      } else {
                        Navigator.of(context).pop();
                      }
                    },
                    child: const Align(
                      alignment: Alignment.centerLeft,
                      child: Icon(Icons.arrow_back_ios_new,
                          size: 20, color: Color(0xFF1A1A1A)),
                    ),
                  ),
                  const SizedBox(height: 16),
                  const Text(
                    'Transaction Pin',
                    style: TextStyle(
                      fontSize: 20,
                      fontWeight: FontWeight.w800,
                      color: Color(0xFF1A1A1A),
                    ),
                  ),
                  const SizedBox(height: 2),
                  const Text(
                    'Manage Transaction Pin and access to your earnings.',
                    style: TextStyle(fontSize: 13, color: Color(0xFF888888)),
                  ),
                ],
              ),
            ),
            const SizedBox(height: 32),
            Text(
              _isConfirming ? 'Confirm New Pin!' : 'Create New Pin!',
              style: const TextStyle(
                fontSize: 18,
                fontWeight: FontWeight.w800,
                color: Color(0xFF1A1A1A),
              ),
            ),
            const SizedBox(height: 24),
            _PinDots(pin: pin),
            const SizedBox(height: 32),
            NumericKeypad(onKeyTap: _onKey),
            const Spacer(),
            Padding(
              padding: const EdgeInsets.fromLTRB(20, 0, 20, 16),
              child: SizedBox(
                height: 52,
                width: double.infinity,
                child: FilledButton(
                  onPressed: _isConfirming && _pinsMatch ? _submit : null,
                  style: FilledButton.styleFrom(
                    backgroundColor: const Color(0xFF4CAF50),
                    disabledBackgroundColor:
                        const Color(0xFF4CAF50).withValues(alpha: 0.35),
                    shape: RoundedRectangleBorder(
                      borderRadius: BorderRadius.circular(999),
                    ),
                  ),
                  child: const Text(
                    'Create Pin',
                    style: TextStyle(
                      fontSize: 16,
                      fontWeight: FontWeight.w700,
                      color: Colors.white,
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

class _PinDots extends StatelessWidget {
  const _PinDots({required this.pin});
  final String pin;

  @override
  Widget build(BuildContext context) {
    return Row(
      mainAxisAlignment: MainAxisAlignment.center,
      children: List.generate(4, (i) {
        final filled = i < pin.length;
        return Container(
          margin: const EdgeInsets.symmetric(horizontal: 10),
          width: 52,
          height: 52,
          alignment: Alignment.center,
          decoration: BoxDecoration(
            shape: BoxShape.circle,
            border: Border.all(
              color: const Color(0xFF4CAF50),
              width: 1.5,
            ),
          ),
          child: Text(
            filled ? pin[i] : '',
            style: const TextStyle(
              fontSize: 20,
              fontWeight: FontWeight.w700,
              color: Color(0xFF4CAF50),
            ),
          ),
        );
      }),
    );
  }
}