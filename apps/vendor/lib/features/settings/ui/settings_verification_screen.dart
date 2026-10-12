import 'dart:async';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import '../../../app/vendor_runtime.dart';
import '../../../core/theme/vendor_theme.dart';
import '../../auth/models/vendor_session.dart';
import '../state/settings_controller.dart';
import 'settings_design.dart';

const settingsStepUpPurpose = 'iam.high_risk_permission_change';

class SettingsVerificationScreen extends StatefulWidget {
  const SettingsVerificationScreen({
    super.key,
    required this.runtime,
    this.purpose = settingsStepUpPurpose,
    this.title = 'Verify access change',
    this.actionLabel = 'Confirm change',
  });
  final VendorRuntime runtime;
  final String purpose, title, actionLabel;
  @override
  State<SettingsVerificationScreen> createState() =>
      _SettingsVerificationScreenState();
}

class _SettingsVerificationScreenState
    extends State<SettingsVerificationScreen> {
  final _code = TextEditingController();
  VendorChallenge? _challenge;
  DateTime? _expires;
  Timer? _timer;
  String? _error, _token;
  bool _sending = false, _verifying = false;
  int get remaining => _expires == null
      ? 0
      : (_expires!.difference(DateTime.now()).inMilliseconds / 1000)
            .ceil()
            .clamp(0, 86400);
  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (mounted) unawaited(_start());
    });
  }

  @override
  void dispose() {
    _timer?.cancel();
    _code.clear();
    _code.dispose();
    super.dispose();
  }

  Future<void> _start() async {
    if (_sending || _verifying) return;
    setState(() {
      _sending = true;
      _error = null;
      _challenge = null;
      _code.clear();
    });
    _timer?.cancel();
    try {
      final challenge = await widget.runtime.auth.startStepUp(widget.purpose);
      if (!mounted) return;
      if (challenge.expiresIn <= 0) throw StateError('Code expired');
      setState(() {
        _challenge = challenge;
        _expires = DateTime.now().add(Duration(seconds: challenge.expiresIn));
        _token = widget.runtime.auth.session?.accessToken;
        if (widget.runtime.mode == VendorDataMode.mock) _code.text = '123456';
      });
      _timer = Timer.periodic(const Duration(seconds: 1), (_) {
        if (!mounted || remaining == 0) _timer?.cancel();
        if (mounted) setState(() {});
      });
    } catch (e) {
      if (mounted) setState(() => _error = SettingsController.message(e));
    } finally {
      if (mounted) setState(() => _sending = false);
    }
  }

  Future<void> _verify() async {
    if (_sending ||
        _verifying ||
        _challenge == null ||
        remaining == 0 ||
        _code.text.length != 6) {
      return;
    }
    setState(() {
      _verifying = true;
      _error = null;
    });
    try {
      if (_token == null ||
          widget.runtime.auth.session?.accessToken != _token) {
        throw StateError('Sign-in session changed');
      }
      final grant = await widget.runtime.auth.verifyStepUp(
        purpose: widget.purpose,
        challengeId: _challenge!.challengeId,
        otp: _code.text,
      );
      if (grant.expiresIn <= 0) throw StateError('Verification expired');
      _code.clear();
      _timer?.cancel();
      if (mounted) {
        setState(() => _verifying = false);
        Navigator.pop(context, grant.grantId);
      }
    } catch (e) {
      if (mounted) setState(() => _error = SettingsController.message(e));
    } finally {
      if (mounted) setState(() => _verifying = false);
    }
  }

  @override
  Widget build(BuildContext context) => PopScope(
    canPop: !_sending && !_verifying,
    child: Scaffold(
      appBar: AppBar(
        title: Text(widget.title),
        backgroundColor: KarryamColors.canvas.forSurface(context),
      ),
      bottomNavigationBar: SafeArea(
        child: Padding(
          padding: const EdgeInsets.fromLTRB(24, 12, 24, 16),
          child: SettingsAction(
            widget.actionLabel,
            icon: Icons.lock_outline_rounded,
            busy: _verifying,
            onPressed:
                !_sending &&
                    remaining > 0 &&
                    _challenge != null &&
                    _code.text.length == 6
                ? _verify
                : null,
          ),
        ),
      ),
      body: ListView(
        padding: const EdgeInsets.all(24),
        children: [
          const SettingsEmblem(SettingsKind.roles, size: 112),
          const SizedBox(height: 24),
          const SettingsHeading(
            'Verify it is you',
            subtitle: 'Enter the code sent to your sign-in contact.',
          ),
          if (_error != null) SettingsNote(_error!),
          TextField(
            controller: _code,
            enabled:
                !_sending && !_verifying && remaining > 0 && _challenge != null,
            keyboardType: TextInputType.number,
            obscureText: true,
            autocorrect: false,
            enableSuggestions: false,
            autofillHints: const [AutofillHints.oneTimeCode],
            inputFormatters: [
              FilteringTextInputFormatter.digitsOnly,
              LengthLimitingTextInputFormatter(6),
            ],
            onChanged: (_) => setState(() {}),
            decoration: const InputDecoration(
              labelText: 'Verification code',
              hintText: '6 digits',
            ),
          ),
          const SizedBox(height: 14),
          if (_challenge != null)
            Text(
              remaining > 0
                  ? 'Expires in ${remaining ~/ 60}:${(remaining % 60).toString().padLeft(2, '0')}'
                  : 'Code expired. Request a new code.',
            ),
          const SizedBox(height: 18),
          TextButton(
            onPressed: _sending || _verifying ? null : _start,
            child: _sending
                ? const SizedBox(
                    width: 20,
                    height: 20,
                    child: CircularProgressIndicator(strokeWidth: 2),
                  )
                : const Text('Request a new code'),
          ),
        ],
      ),
    ),
  );
}
