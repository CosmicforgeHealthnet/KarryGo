import 'package:flutter/material.dart';

import '../../home/ui/widgets/provider_app_colors.dart';

// ─── Progress bar ─────────────────────────────────────────────────────────────

class OnboardingProgressBar extends StatelessWidget {
  const OnboardingProgressBar({super.key, required this.step, this.total = 7});

  final int step;
  final int total;

  @override
  Widget build(BuildContext context) {
    return Row(
      children: List.generate(total, (i) {
        final filled = i < step;
        return Expanded(
          child: Container(
            height: 4,
            margin: EdgeInsets.only(right: i < total - 1 ? 4 : 0),
            decoration: BoxDecoration(
              color: filled ? kProviderGreen : kProviderBorder,
              borderRadius: BorderRadius.circular(2),
            ),
          ),
        );
      }),
    );
  }
}

// ─── Upload area ──────────────────────────────────────────────────────────────

class UploadArea extends StatelessWidget {
  const UploadArea({
    super.key,
    required this.label,
    required this.subtitle,
    required this.onTap,
    this.fileName,
  });

  final String label;
  final String subtitle;
  final VoidCallback onTap;
  final String? fileName;

  @override
  Widget build(BuildContext context) {
    final uploaded = fileName != null;
    return GestureDetector(
      onTap: onTap,
      child: AnimatedContainer(
        duration: const Duration(milliseconds: 150),
        padding: const EdgeInsets.all(16),
        decoration: BoxDecoration(
          color: uploaded ? kProviderGreenTint : Colors.white,
          borderRadius: BorderRadius.circular(12),
          border: Border.all(
            color: uploaded ? kProviderGreen : kProviderBorder,
            width: uploaded ? 1.5 : 1,
            style: uploaded ? BorderStyle.solid : BorderStyle.solid,
          ),
        ),
        child: Row(
          children: [
            Container(
              width: 44,
              height: 44,
              decoration: BoxDecoration(
                color: uploaded ? kProviderGreen : kProviderSurface,
                borderRadius: BorderRadius.circular(10),
              ),
              child: Icon(
                uploaded ? Icons.check_rounded : Icons.upload_file_rounded,
                color: uploaded ? Colors.white : kProviderMuted,
                size: 22,
              ),
            ),
            const SizedBox(width: 12),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    label,
                    style: TextStyle(
                      color: kProviderText,
                      fontWeight: FontWeight.w700,
                      fontSize: 13,
                    ),
                  ),
                  const SizedBox(height: 2),
                  Text(
                    uploaded ? fileName! : subtitle,
                    style: TextStyle(
                      color: uploaded ? kProviderGreen : kProviderMuted,
                      fontSize: 12,
                    ),
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis,
                  ),
                ],
              ),
            ),
            const SizedBox(width: 8),
            Icon(
              Icons.arrow_forward_ios_rounded,
              color: kProviderMuted,
              size: 14,
            ),
          ],
        ),
      ),
    );
  }
}

// ─── Section label ────────────────────────────────────────────────────────────

class OnboardingSectionLabel extends StatelessWidget {
  const OnboardingSectionLabel(this.text, {super.key});
  final String text;

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.only(bottom: 6),
      child: Text(
        text,
        style: const TextStyle(
          color: kProviderText,
          fontSize: 13,
          fontWeight: FontWeight.w700,
        ),
      ),
    );
  }
}

// ─── Field widget ─────────────────────────────────────────────────────────────

InputDecoration onboardingFieldDecoration(String hint, {Widget? prefix}) =>
    InputDecoration(
      hintText: hint,
      hintStyle: const TextStyle(color: kProviderMuted, fontSize: 14),
      filled: true,
      fillColor: Colors.white,
      prefixIcon: prefix,
      border: OutlineInputBorder(
        borderRadius: BorderRadius.circular(12),
        borderSide: const BorderSide(color: kProviderBorder),
      ),
      enabledBorder: OutlineInputBorder(
        borderRadius: BorderRadius.circular(12),
        borderSide: const BorderSide(color: kProviderBorder),
      ),
      focusedBorder: OutlineInputBorder(
        borderRadius: BorderRadius.circular(12),
        borderSide: const BorderSide(color: kProviderGreen, width: 2),
      ),
      contentPadding: const EdgeInsets.symmetric(horizontal: 14, vertical: 14),
    );

// ─── Scaffold ─────────────────────────────────────────────────────────────────

class OnboardingScaffold extends StatelessWidget {
  const OnboardingScaffold({
    super.key,
    required this.title,
    this.subtitle,
    this.step,
    required this.content,
    this.onContinue,
    this.continueLabel = 'Continue',
    this.isLoading = false,
    this.error,
    this.onBack,
    this.showBack = true,
  });

  final String title;
  final String? subtitle;
  final int? step;
  final Widget content;
  final VoidCallback? onContinue;
  final String continueLabel;
  final bool isLoading;
  final String? error;
  final VoidCallback? onBack;
  final bool showBack;

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: kProviderSurface,
      appBar: AppBar(
        backgroundColor: kProviderSurface,
        elevation: 0,
        leading: showBack
            ? IconButton(
                icon: const Icon(Icons.arrow_back_ios_new_rounded, color: kProviderText, size: 20),
                onPressed: onBack ?? () => Navigator.maybePop(context),
              )
            : const SizedBox.shrink(),
        automaticallyImplyLeading: false,
      ),
      body: SafeArea(
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            if (step != null)
              Padding(
                padding: const EdgeInsets.fromLTRB(24, 0, 24, 16),
                child: OnboardingProgressBar(step: step!),
              ),
            Expanded(
              child: SingleChildScrollView(
                padding: const EdgeInsets.fromLTRB(24, 0, 24, 16),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.stretch,
                  children: [
                    Text(
                      title,
                      style: const TextStyle(
                        color: kProviderText,
                        fontSize: 22,
                        fontWeight: FontWeight.w800,
                        height: 1.2,
                      ),
                    ),
                    if (subtitle != null) ...[
                      const SizedBox(height: 8),
                      Text(
                        subtitle!,
                        style: const TextStyle(color: kProviderMuted, fontSize: 13, height: 1.5),
                      ),
                    ],
                    const SizedBox(height: 24),
                    content,
                  ],
                ),
              ),
            ),
            Padding(
              padding: const EdgeInsets.fromLTRB(24, 0, 24, 24),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  if (error != null) ...[
                    Text(error!, style: const TextStyle(color: Colors.red, fontSize: 12), textAlign: TextAlign.center),
                    const SizedBox(height: 8),
                  ],
                  SizedBox(
                    height: 52,
                    child: FilledButton(
                      onPressed: onContinue,
                      style: FilledButton.styleFrom(
                        backgroundColor: kProviderGreen,
                        disabledBackgroundColor: kProviderGreen.withValues(alpha: 0.4),
                        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(999)),
                      ),
                      child: isLoading
                          ? const SizedBox.square(
                              dimension: 20,
                              child: CircularProgressIndicator(strokeWidth: 2, color: Colors.white),
                            )
                          : Text(
                              continueLabel,
                              style: const TextStyle(color: Colors.white, fontWeight: FontWeight.w700, fontSize: 16),
                            ),
                    ),
                  ),
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }
}
