import 'package:flutter/material.dart';

import '../../../core/widgets/karryam_ui.dart';
import '../../home/ui/widgets/provider_app_colors.dart';

// ─── Progress bar ─────────────────────────────────────────────────────────────

class OnboardingProgressBar extends StatelessWidget {
  const OnboardingProgressBar({super.key, required this.step, this.total = 7});

  final int step;
  final int total;

  @override
  Widget build(BuildContext context) => KProgressSteps(total: total, current: step);
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
    return KCard(
      onTap: onTap,
      borderColor: uploaded ? kTeal : kProviderBorder,
      color: uploaded ? kSelectedWash : kCard,
      child: Row(
        children: [
          KIconTile(
            uploaded ? Icons.check_rounded : Icons.add_photo_alternate_outlined,
            color: uploaded ? kNavy : kTealText,
            background: uploaded ? kTeal : kTealTint,
            size: 44,
          ),
          const SizedBox(width: 12),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(label, style: TextStyle(color: kProviderText, fontWeight: FontWeight.w600, fontSize: 14)),
                const SizedBox(height: 2),
                Text(
                  uploaded ? fileName! : subtitle,
                  style: TextStyle(color: uploaded ? kTealText : kProviderMuted, fontSize: 12.5),
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                ),
              ],
            ),
          ),
          const SizedBox(width: 8),
          Text(
            uploaded ? 'Change' : 'Upload',
            style: TextStyle(color: kTealText, fontSize: 13, fontWeight: FontWeight.w600),
          ),
        ],
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
      padding: const EdgeInsets.only(bottom: 8),
      child: Text(
        text,
        style: TextStyle(color: kProviderText, fontSize: 13, fontWeight: FontWeight.w600),
      ),
    );
  }
}

// ─── Field widget ─────────────────────────────────────────────────────────────

InputDecoration onboardingFieldDecoration(String hint, {Widget? prefix}) =>
    InputDecoration(
      hintText: hint,
      hintStyle: TextStyle(color: kProviderMuted, fontSize: 14),
      filled: true,
      fillColor: kCard,
      prefixIcon: prefix,
      border: OutlineInputBorder(
        borderRadius: BorderRadius.circular(12),
        borderSide: BorderSide(color: kProviderBorder),
      ),
      enabledBorder: OutlineInputBorder(
        borderRadius: BorderRadius.circular(12),
        borderSide: BorderSide(color: kProviderBorder),
      ),
      focusedBorder: OutlineInputBorder(
        borderRadius: BorderRadius.circular(12),
        borderSide: const BorderSide(color: kTeal, width: 1.6),
      ),
      contentPadding: const EdgeInsets.symmetric(horizontal: 16, vertical: 16),
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
      appBar: KTopBar(title: 'Karryam Trucks', showBack: showBack, onBack: onBack),
      body: SafeArea(
        top: false,
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            if (step != null)
              Padding(
                padding: const EdgeInsets.fromLTRB(24, 4, 24, 20),
                child: OnboardingProgressBar(step: step!),
              ),
            Expanded(
              child: SingleChildScrollView(
                padding: const EdgeInsets.fromLTRB(24, 0, 24, 16),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.stretch,
                  children: [
                    KHeading(title: title, subtitle: subtitle),
                    const SizedBox(height: 24),
                    content,
                  ],
                ),
              ),
            ),
            Padding(
              padding: const EdgeInsets.fromLTRB(24, 8, 24, 16),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  if (error != null) ...[
                    KNotice(error!, tone: KTone.danger),
                    const SizedBox(height: 12),
                  ],
                  KPrimaryButton(
                    label: continueLabel,
                    arrow: true,
                    loading: isLoading,
                    onPressed: onContinue,
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
