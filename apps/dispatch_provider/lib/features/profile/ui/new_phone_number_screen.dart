import '../../../shared/widgets/dispatch_screen_frame.dart';
import '../../../shared/widgets/journey_icon.dart';
import 'dart:async';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import '../../../core/app_mode.dart';
import '../../../core/network/provider_session.dart';
import '../../../core/theme/theme_extensions.dart';
import '../../../shared/widgets/async_content.dart';
import '../../auth/data/dispatch_auth_api.dart';

class NewPhoneNumberScreen extends StatefulWidget {
  const NewPhoneNumberScreen({
    super.key,
    required this.countryFlag,
    required this.countryCode,
  });
  final String countryFlag;
  final String countryCode;
  @override
  State<NewPhoneNumberScreen> createState() => _NewPhoneNumberScreenState();
}

class _NewPhoneNumberScreenState extends State<NewPhoneNumberScreen> {
  final _phone = TextEditingController();
  final _code = TextEditingController();
  bool _sent = false;
  bool _busy = false;
  bool _confirmed = false;
  String? _error;
  DateTime? _expiresAt;
  DateTime? _resendAt;
  Timer? _timer;
  String get _number =>
      DispatchAuthApi.normalizeNigerianPhoneNumber(_phone.text);
  int get _cooldown =>
      (_resendAt?.difference(DateTime.now()).inSeconds ?? 0).clamp(0, 30);

  Future<void> _submit({bool resend = false}) async {
    if (_busy) return;
    if (!_sent && !RegExp(r'^\+[1-9]\d{7,14}$').hasMatch(_number)) {
      setState(
        () => _error = 'Enter a valid number, including the country code.',
      );
      return;
    }
    if (_sent &&
        !resend &&
        (_code.text.length != 6 ||
            (_expiresAt != null && DateTime.now().isAfter(_expiresAt!)))) {
      setState(
        () => _error =
            'Enter the six-digit code, or request a new code if it expired.',
      );
      return;
    }
    final session = ProviderSession.of(context);
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      final verifying = _sent && !resend;
      final data = await session.command(
        '/api/v1/auth/phone-change/${verifying ? 'verify' : 'start'}',
        {'new_phone_number': _number, if (verifying) 'otp_code': _code.text},
      );
      if (!mounted) return;
      if (verifying) {
        setState(() => _confirmed = true);
        await session.refreshProfile();
      } else {
        final seconds = (data['expires_in_seconds'] as num?)?.toInt() ?? 300;
        setState(() {
          _sent = true;
          _code.clear();
          _expiresAt = DateTime.now().add(Duration(seconds: seconds));
          _resendAt = DateTime.now().add(const Duration(seconds: 30));
        });
        _timer?.cancel();
        _timer = Timer.periodic(const Duration(seconds: 1), (_) {
          if (mounted) setState(() {});
        });
      }
    } catch (error) {
      if (mounted)
        setState(
          () => _error = error is ApiException
              ? error.message
              : 'We could not confirm the change. Check your connection and try again.',
        );
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  void dispose() {
    _timer?.cancel();
    _phone.dispose();
    _code.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    return Scaffold(
      appBar: AppBar(
        leading: const DispatchBackButton(),
        title: const Text('Phone number'),
      ),
      body: _confirmed
          ? ConfirmedResult(
              title: 'Phone number updated',
              message: 'You can now sign in with $_number.',
              onDone: () => Navigator.of(context).pop({
                'flag': widget.countryFlag,
                'code': widget.countryCode,
                'number': _number,
              }),
            )
          : SafeArea(
              child: SingleChildScrollView(
                padding: const EdgeInsets.all(24),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.stretch,
                  children: [
                    Align(
                      alignment: Alignment.centerLeft,
                      child: Container(
                        padding: const EdgeInsets.all(18),
                        decoration: BoxDecoration(
                          color: c.teal10,
                          borderRadius: BorderRadius.circular(24),
                        ),
                        child: JourneyIcon(
                          _sent ? JourneySymbol.key : JourneySymbol.contact,
                          size: 32,
                          color: c.brandTeal600,
                        ),
                      ),
                    ),
                    const SizedBox(height: 24),
                    Text(
                      _sent ? 'Verify your new number' : 'Change phone number',
                      style: Theme.of(context).textTheme.headlineMedium
                          ?.copyWith(fontWeight: FontWeight.w700),
                    ),
                    const SizedBox(height: 12),
                    Text(
                      _sent
                          ? 'Enter the code sent to $_number.'
                          : 'Verify a new number before changing your sign-in details.',
                      style: TextStyle(color: c.textMuted, height: 1.5),
                    ),
                    const SizedBox(height: 28),
                    AnimatedSwitcher(
                      duration: const Duration(milliseconds: 220),
                      child: _sent
                          ? TextField(
                              key: const ValueKey('code'),
                              controller: _code,
                              keyboardType: TextInputType.number,
                              maxLength: 6,
                              inputFormatters: [
                                FilteringTextInputFormatter.digitsOnly,
                              ],
                              autofillHints: const [AutofillHints.oneTimeCode],
                              enabled: !_busy,
                              decoration: const InputDecoration(
                                labelText: 'Verification code',
                              ),
                            )
                          : TextField(
                              key: const ValueKey('phone'),
                              controller: _phone,
                              keyboardType: TextInputType.phone,
                              enabled: !_busy,
                              autofillHints: const [
                                AutofillHints.telephoneNumber,
                              ],
                              decoration: const InputDecoration(
                                labelText: 'New phone number',
                                hintText: '+234...',
                              ),
                            ),
                    ),
                    if (_sent && AppModeConfig.instance.isMock) ...[
                      const SizedBox(height: 12),
                      const ActionNotice(message: 'Demo code: 123456'),
                    ],
                    if (_error != null) ...[
                      const SizedBox(height: 16),
                      ActionNotice(message: _error!, isError: true),
                    ],
                    const SizedBox(height: 24),
                    FilledButton(
                      onPressed: _busy ? null : _submit,
                      child: _busy
                          ? const SizedBox(
                              width: 20,
                              height: 20,
                              child: CircularProgressIndicator(strokeWidth: 2),
                            )
                          : Text(
                              _sent
                                  ? 'Verify number'
                                  : 'Send verification code',
                            ),
                    ),
                    if (_sent) ...[
                      TextButton(
                        onPressed: _busy || _cooldown > 0
                            ? null
                            : () => _submit(resend: true),
                        child: Text(
                          _cooldown > 0
                              ? 'Resend in ${_cooldown}s'
                              : 'Request a new code',
                        ),
                      ),
                      TextButton(
                        onPressed: _busy
                            ? null
                            : () => setState(() {
                                _sent = false;
                                _error = null;
                              }),
                        child: const Text('Use a different number'),
                      ),
                    ],
                  ],
                ),
              ),
            ),
    );
  }
}
