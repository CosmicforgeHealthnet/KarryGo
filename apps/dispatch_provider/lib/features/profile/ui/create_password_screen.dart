import 'package:flutter/material.dart';
import 'package:dispatch_provider/shared/widgets/password_widgets.dart';

class CreatePasswordScreen extends StatefulWidget {
  const CreatePasswordScreen({super.key});

  @override
  State<CreatePasswordScreen> createState() => _CreatePasswordScreenState();
}

class _CreatePasswordScreenState extends State<CreatePasswordScreen> {
  final _newPasswordController = TextEditingController();
  final _confirmPasswordController = TextEditingController();

  bool _newObscure = true;
  bool _confirmObscure = true;

  bool get _hasMinLength => _newPasswordController.text.length >= 8;
  bool get _hasUppercase =>
      _newPasswordController.text.contains(RegExp(r'[A-Z]'));
  bool get _hasLowercase =>
      _newPasswordController.text.contains(RegExp(r'[a-z]'));
  bool get _hasNumberOrSpecial =>
      _newPasswordController.text.contains(RegExp(r'[0-9!@#\$&*~_]'));

  bool get _newPasswordValid =>
      _hasMinLength && _hasUppercase && _hasLowercase && _hasNumberOrSpecial;

  bool get _confirmMatches =>
      _confirmPasswordController.text == _newPasswordController.text &&
      _confirmPasswordController.text.isNotEmpty;

  bool get _canSubmit => _newPasswordValid && _confirmMatches;

  Color _newBorderColor() {
    if (_newPasswordController.text.isEmpty) return const Color(0xFFEEEEEE);
    return _newPasswordValid
        ? const Color(0xFF05124D)
        : const Color(0xFFE53935);
  }

  Color _confirmBorderColor() {
    if (_confirmPasswordController.text.isEmpty) return const Color(0xFFEEEEEE);
    if (_confirmMatches) return const Color(0xFF05124D);
    return const Color(0xFFFFA000);
  }

  Color get _requirementsBg =>
      _newPasswordValid ? const Color(0xFFE7F5EC) : const Color(0xFFFDECEC);

  Color get _requirementsTextColor =>
      _newPasswordValid ? const Color(0xFF2E7D32) : const Color(0xFFE53935);

  @override
  void dispose() {
    _newPasswordController.dispose();
    _confirmPasswordController.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: Colors.white,
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
                      child: Icon(Icons.arrow_back_ios_new,
                          size: 20, color: Color(0xFF1A1A1A)),
                    ),
                  ),
                  const SizedBox(height: 16),
                  const Text(
                    'Passwords',
                    style: TextStyle(
                      fontSize: 20,
                      fontWeight: FontWeight.w800,
                      color: Color(0xFF1A1A1A),
                    ),
                  ),
                  const SizedBox(height: 2),
                  const Text(
                    'Manage passwords and access to your account.',
                    style: TextStyle(fontSize: 13, color: Color(0xFF888888)),
                  ),
                  const SizedBox(height: 24),
                  const Text(
                    'Create New Password!',
                    style: TextStyle(
                      fontSize: 18,
                      fontWeight: FontWeight.w800,
                      color: Color(0xFF1A1A1A),
                    ),
                  ),
                  const SizedBox(height: 20),
                  const PasswordFieldLabel('New Password'),
                  const SizedBox(height: 8),
                  PasswordField(
                    controller: _newPasswordController,
                    hint: 'Enter New Password',
                    obscure: _newObscure,
                    borderColor: _newBorderColor(),
                    onToggleObscure: () =>
                        setState(() => _newObscure = !_newObscure),
                    onChanged: (_) => setState(() {}),
                  ),
                  const SizedBox(height: 16),
                  const PasswordFieldLabel('Confirm New Password'),
                  const SizedBox(height: 8),
                  PasswordField(
                    controller: _confirmPasswordController,
                    hint: 'Confirm Password',
                    obscure: _confirmObscure,
                    borderColor: _confirmBorderColor(),
                    onToggleObscure: () =>
                        setState(() => _confirmObscure = !_confirmObscure),
                    onChanged: (_) => setState(() {}),
                  ),
                  const SizedBox(height: 20),
                  const PasswordFieldLabel('Password Requirement'),
                  const SizedBox(height: 8),
                  Container(
                    padding: const EdgeInsets.all(14),
                    decoration: BoxDecoration(
                      color: _requirementsBg,
                      borderRadius: BorderRadius.circular(10),
                    ),
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        PasswordRequirement(
                            'At least 8 characters long', _requirementsTextColor),
                        PasswordRequirement(
                            'Contains at least one uppercase letter',
                            _requirementsTextColor),
                        PasswordRequirement(
                            'Contains at least one lowercase letter',
                            _requirementsTextColor),
                        PasswordRequirement(
                            'Contains at least one number or special character',
                            _requirementsTextColor),
                      ],
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
                  onPressed: _canSubmit ? _submit : null,
                  style: FilledButton.styleFrom(
                    backgroundColor: const Color(0xFF05124D),
                    disabledBackgroundColor:
                        const Color(0xFF05124D).withValues(alpha: 0.35),
                    shape: RoundedRectangleBorder(
                      borderRadius: BorderRadius.circular(999),
                    ),
                  ),
                  child: const Text(
                    'Create Password',
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

  void _submit() {
    // TODO: wire to API
    Navigator.of(context).pop();
  }
}