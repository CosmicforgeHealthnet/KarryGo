import '../../../shared/widgets/account_components.dart';
import '../../../shared/widgets/async_content.dart';
import '../../../shared/widgets/journey_icon.dart';
import '../../../shared/widgets/dispatch_screen_frame.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../state/wallet_controller.dart';

class ChangeTransactionPinScreen extends StatefulWidget {
  const ChangeTransactionPinScreen({super.key, required this.walletController});

  final WalletController walletController;

  @override
  State<ChangeTransactionPinScreen> createState() =>
      _ChangeTransactionPinScreenState();
}

class _ChangeTransactionPinScreenState
    extends State<ChangeTransactionPinScreen> {
  final _currentController = TextEditingController();
  final _newController = TextEditingController();
  final _confirmController = TextEditingController();

  bool _submitting = false;
  String? _error;
  bool _success = false;

  @override
  void dispose() {
    _currentController.dispose();
    _newController.dispose();
    _confirmController.dispose();
    super.dispose();
  }

  bool get _isValid {
    if (_currentController.text.length != 6) return false;
    if (_newController.text.length != 6) return false;
    if (_confirmController.text.length != 6) return false;
    if (_newController.text != _confirmController.text) return false;
    return true;
  }

  Future<void> _submit() async {
    if (!_isValid || _submitting) return;

    setState(() {
      _submitting = true;
      _error = null;
    });

    try {
      final result = await widget.walletController.changePin(
        currentPin: _currentController.text,
        newPin: _newController.text,
      );
      if (!mounted) return;
      result.when(
        success: (_) => setState(() {
          _submitting = false;
          _success = true;
        }),
        failure: (error) => setState(() {
          _submitting = false;
          _error = error.message;
        }),
      );
    } catch (e) {
      if (!mounted) return;
      setState(() {
        _submitting = false;
        _error = 'Could not change your PIN. Please try again.';
      });
    }
  }

  @override
  Widget build(BuildContext context) {
    if (_success)
      return Scaffold(
        appBar: AppBar(leading: const DispatchBackButton()),
        body: ConfirmedResult(
          title: 'Withdrawal PIN updated',
          message: 'Use your new PIN for your next withdrawal.',
          onDone: () => Navigator.of(context).pop(),
        ),
      );
    return AccountPage(
      title: 'Change withdrawal PIN',
      symbol: JourneySymbol.key,
      description: 'Keep your six-digit PIN private.',
      bottom: FilledButton(
        onPressed: _isValid && !_submitting ? _submit : null,
        child: _submitting
            ? const SizedBox.square(
                dimension: 20,
                child: CircularProgressIndicator(strokeWidth: 2),
              )
            : const Text('Save new PIN'),
      ),
      children: [
        for (final field in [
          (_currentController, 'Current PIN'),
          (_newController, 'New PIN'),
          (_confirmController, 'Confirm new PIN'),
        ])
          Padding(
            padding: const EdgeInsets.only(bottom: 20),
            child: TextField(
              controller: field.$1,
              enabled: !_submitting,
              obscureText: true,
              enableSuggestions: false,
              autocorrect: false,
              keyboardType: TextInputType.number,
              inputFormatters: [
                FilteringTextInputFormatter.digitsOnly,
                LengthLimitingTextInputFormatter(6),
              ],
              onChanged: (_) => setState(() {}),
              decoration: InputDecoration(labelText: field.$2),
            ),
          ),
        if (_confirmController.text.length == 6 &&
            _newController.text != _confirmController.text)
          const ActionNotice(
            message: 'The new PINs do not match.',
            isError: true,
          ),
        if (_error != null) ActionNotice(message: _error!, isError: true),
      ],
    );
  }
}
