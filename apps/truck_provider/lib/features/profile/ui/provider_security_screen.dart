import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../../home/ui/widgets/provider_app_colors.dart';
import '../../wallet/state/provider_withdrawal_controller.dart';
import 'widgets/provider_profile_widgets.dart';

/// Security: the transaction PIN that approves every withdrawal. Sign-in
/// itself uses one-time codes, so the PIN is the only secret to manage.
class ProviderSecurityScreen extends StatefulWidget {
  const ProviderSecurityScreen({super.key, required this.controller});

  final ProviderWithdrawalController controller;

  @override
  State<ProviderSecurityScreen> createState() => _ProviderSecurityScreenState();
}

class _ProviderSecurityScreenState extends State<ProviderSecurityScreen> {
  @override
  void initState() {
    super.initState();
    widget.controller.loadPinStatus();
  }

  void _toast(String message) => ScaffoldMessenger.of(context)
    ..hideCurrentSnackBar()
    ..showSnackBar(SnackBar(content: Text(message), behavior: SnackBarBehavior.floating));

  Future<void> _setOrChange(bool hasPin) async {
    final c = widget.controller;
    String? current;
    if (hasPin) {
      current = await _askPin('Enter your current PIN');
      if (current == null) return;
    }
    final first = await _askPin(hasPin ? 'Choose a new PIN' : 'Create a 4-digit PIN');
    if (first == null) return;
    final second = await _askPin('Enter the new PIN again');
    if (second == null) return;
    if (first != second) {
      _toast('The two PINs did not match. Nothing was changed.');
      return;
    }
    final ok = hasPin ? await c.changePin(currentPin: current!, newPin: first) : await c.setupPin(first);
    if (!mounted) return;
    _toast(ok ? (hasPin ? 'Your PIN was changed.' : 'Your PIN is set.') : (c.pinError ?? 'Something went wrong.'));
  }

  Future<String?> _askPin(String title) =>
      showDialog<String>(context: context, builder: (_) => _PinDialog(title: title));

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      body: SafeArea(
        child: AnimatedBuilder(
          animation: widget.controller,
          builder: (context, _) {
            final pinSet = widget.controller.pinSet;
            return ListView(
              padding: const EdgeInsets.fromLTRB(20, 16, 20, 24),
              children: [
                const ProviderProfileHeader(title: 'Security', subtitle: 'Protect your withdrawals'),
                const SizedBox(height: 24),
                Container(
                  padding: const EdgeInsets.all(16),
                  decoration: BoxDecoration(
                    color: kProviderGreenTint,
                    borderRadius: BorderRadius.circular(16),
                  ),
                  child: Row(
                    children: [
                      Icon(Icons.password_rounded, color: kProviderGreen),
                      const SizedBox(width: 12),
                      Expanded(
                        child: Column(
                          crossAxisAlignment: CrossAxisAlignment.start,
                          children: [
                            Text('Transaction PIN',
                                style: TextStyle(color: kProviderText, fontWeight: FontWeight.w800, fontSize: 16)),
                            const SizedBox(height: 4),
                            Text(
                              pinSet == null
                                  ? 'Checking…'
                                  : pinSet
                                  ? 'Set. You enter it to approve every withdrawal.'
                                  : 'Not set yet. You will need one to withdraw.',
                              style: TextStyle(color: kProviderMuted, fontSize: 13),
                            ),
                          ],
                        ),
                      ),
                    ],
                  ),
                ),
                const SizedBox(height: 20),
                ProviderPrimaryButton(
                  label: pinSet == true ? 'Change PIN' : 'Create PIN',
                  isLoading: widget.controller.savingPin,
                  onPressed: pinSet == null ? null : () => _setOrChange(pinSet),
                ),
                const SizedBox(height: 16),
                Text(
                  'After 5 wrong attempts the PIN locks for 15 minutes. If you forget it, contact support from your profile.',
                  style: TextStyle(color: kProviderMuted, fontSize: 12, height: 1.4),
                ),
              ],
            );
          },
        ),
      ),
    );
  }
}

/// Asks for a 4-digit PIN. Owns its text field so the field outlives the
/// dialog's closing animation.
class _PinDialog extends StatefulWidget {
  const _PinDialog({required this.title});
  final String title;

  @override
  State<_PinDialog> createState() => _PinDialogState();
}

class _PinDialogState extends State<_PinDialog> {
  final _field = TextEditingController();

  @override
  void dispose() {
    _field.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return AlertDialog(
      title: Text(widget.title, style: TextStyle(color: kProviderText, fontWeight: FontWeight.w800, fontSize: 18)),
      content: TextField(
        controller: _field,
        autofocus: true,
        obscureText: true,
        maxLength: 4,
        textAlign: TextAlign.center,
        keyboardType: TextInputType.number,
        inputFormatters: [FilteringTextInputFormatter.digitsOnly],
        style: const TextStyle(fontSize: 24, letterSpacing: 12),
      ),
      actions: [
        TextButton(onPressed: () => Navigator.pop(context), child: const Text('Cancel')),
        TextButton(
          onPressed: () {
            if (_field.text.length == 4) Navigator.pop(context, _field.text);
          },
          child: Text('Continue', style: TextStyle(color: kProviderGreen)),
        ),
      ],
    );
  }
}
