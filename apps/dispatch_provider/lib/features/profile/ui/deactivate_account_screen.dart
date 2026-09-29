import 'package:flutter/material.dart';

import '../../../core/theme/theme_extensions.dart';
import '../state/provider_profile_controller.dart';

class DeactivateAccountScreen extends StatefulWidget {
  const DeactivateAccountScreen({
    super.key,
    required this.profileController,
    this.onDeleted,
  });

  final ProviderProfileController profileController;
  final VoidCallback? onDeleted;

  @override
  State<DeactivateAccountScreen> createState() =>
      _DeactivateAccountScreenState();
}

class _DeactivateAccountScreenState extends State<DeactivateAccountScreen> {
  bool _isSubmitting = false;

  Future<void> _confirm() async {
    if (_isSubmitting) return;
    setState(() => _isSubmitting = true);

    final result = await widget.profileController.deleteAccount();

    if (!mounted) return;
    setState(() => _isSubmitting = false);

    String? error;
    result.when(
      success: (_) {},
      failure: (e) => error = e.message,
    );

    if (error != null) {
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text(error!)),
      );
      return;
    }

    widget.onDeleted?.call();
    Navigator.of(context).pop();
  }

  @override
  Widget build(BuildContext context) {
    final c = context.palette;

    return Scaffold(
      backgroundColor: c.background,
      body: SafeArea(
        child: Column(
          children: [
            // Header
            Padding(
              padding: const EdgeInsets.fromLTRB(12, 8, 12, 4),
              child: Row(
                children: [
                  GestureDetector(
                    onTap: () => Navigator.of(context).pop(),
                    behavior: HitTestBehavior.opaque,
                    child: Padding(
                      padding: const EdgeInsets.all(8),
                      child: Icon(
                        Icons.arrow_back_rounded,
                        size: 22,
                        color: c.text,
                      ),
                    ),
                  ),
                ],
              ),
            ),

            Expanded(
              child: ListView(
                padding: const EdgeInsets.fromLTRB(20, 24, 20, 24),
                children: [
                  const SizedBox(height: 8),

                  // Red warning triangle
                  Center(
                    child: Container(
                      width: 88,
                      height: 88,
                      alignment: Alignment.center,
                      decoration: BoxDecoration(
                        shape: BoxShape.circle,
                        color: c.dangerSoft,
                      ),
                      child: Icon(
                        Icons.warning_amber_rounded,
                        size: 46,
                        color: c.danger,
                      ),
                    ),
                  ),
                  const SizedBox(height: 24),

                  Text(
                    'Deactivate account',
                    textAlign: TextAlign.center,
                    style: TextStyle(
                      fontSize: 24,
                      fontWeight: FontWeight.w800,
                      color: c.text,
                      letterSpacing: -0.4,
                    ),
                  ),
                  const SizedBox(height: 12),

                  Padding(
                    padding: const EdgeInsets.symmetric(horizontal: 8),
                    child: Text(
                      'Are you sure you want to deactivate your account? '
                      'You can reactivate it at any time by signing in again.',
                      textAlign: TextAlign.center,
                      style: TextStyle(
                        fontSize: 15,
                        fontWeight: FontWeight.w500,
                        color: c.textMuted,
                        height: 1.5,
                      ),
                    ),
                  ),
                ],
              ),
            ),

            // Red primary CTA
            Padding(
              padding: const EdgeInsets.fromLTRB(20, 8, 20, 8),
              child: SizedBox(
                height: 52,
                width: double.infinity,
                child: FilledButton(
                  onPressed: _isSubmitting ? null : _confirm,
                  style: FilledButton.styleFrom(
                    backgroundColor: c.danger,
                    foregroundColor: c.brandWhite,
                    disabledBackgroundColor:
                        c.danger.withValues(alpha: 0.35),
                    disabledForegroundColor:
                        c.brandWhite.withValues(alpha: 0.7),
                    shape: const StadiumBorder(),
                    textStyle: const TextStyle(
                      fontSize: 16,
                      fontWeight: FontWeight.w700,
                    ),
                  ),
                  child: _isSubmitting
                      ? SizedBox(
                          width: 20,
                          height: 20,
                          child: CircularProgressIndicator(
                            strokeWidth: 2,
                            color: c.brandWhite,
                          ),
                        )
                      : const Text('Yes, deactivate account'),
                ),
              ),
            ),

            // Cancel — plain text
            Padding(
              padding: const EdgeInsets.fromLTRB(20, 0, 20, 16),
              child: SizedBox(
                height: 44,
                width: double.infinity,
                child: TextButton(
                  onPressed: _isSubmitting
                      ? null
                      : () => Navigator.of(context).pop(),
                  style: TextButton.styleFrom(
                    foregroundColor: c.text,
                    textStyle: const TextStyle(
                      fontSize: 15,
                      fontWeight: FontWeight.w700,
                    ),
                  ),
                  child: const Text('Cancel'),
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }
}