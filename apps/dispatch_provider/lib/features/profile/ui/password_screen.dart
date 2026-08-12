import 'package:flutter/material.dart';
import 'create_password_screen.dart';
import 'change_password_screen.dart';

class PasswordScreen extends StatefulWidget {
  const PasswordScreen({super.key});

  @override
  State<PasswordScreen> createState() => _PasswordScreenState();
}

class _PasswordScreenState extends State<PasswordScreen> {
  bool _hasPassword = false;
  bool _enabled = false;

  void _onToggle(bool value) {
    setState(() => _enabled = value);
    if (value) {
      Navigator.of(context).push(
        MaterialPageRoute(builder: (_) => const CreatePasswordScreen()),
      );
    }
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
                  Container(
                    padding: const EdgeInsets.symmetric(
                        horizontal: 16, vertical: 14),
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
                                'Password',
                                style: TextStyle(
                                  fontSize: 14,
                                  fontWeight: FontWeight.w700,
                                  color: Color(0xFF1A1A1A),
                                ),
                              ),
                              SizedBox(height: 2),
                              Text(
                                'Do you want to add password to your account.',
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
            Padding(
              padding: const EdgeInsets.fromLTRB(20, 0, 20, 16),
              child: SizedBox(
                height: 52,
                width: double.infinity,
                child: FilledButton(
                  onPressed: _enabled
                      ? () {
                          if (_hasPassword) {
                            Navigator.of(context).push(
                              MaterialPageRoute(
                                  builder: (_) =>
                                      const ChangePasswordScreen()),
                            );
                          } else {
                            Navigator.of(context).push(
                              MaterialPageRoute(
                                  builder: (_) =>
                                      const CreatePasswordScreen()),
                            );
                          }
                        }
                      : null,
                  style: FilledButton.styleFrom(
                    backgroundColor: const Color(0xFF05124D),
                    disabledBackgroundColor:
                        const Color(0xFF05124D).withValues(alpha: 0.35),
                    shape: RoundedRectangleBorder(
                      borderRadius: BorderRadius.circular(999),
                    ),
                  ),
                  child: Text(
                    _hasPassword ? 'Manage Password' : 'Create Password',
                    style: const TextStyle(
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