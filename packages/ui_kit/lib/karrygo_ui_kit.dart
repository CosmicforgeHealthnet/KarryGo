library;

import 'package:flutter/material.dart';

class KarryGoColors {
  static const primary = Color(0xFF20AD4E);
  static const primaryDark = Color(0xFF0D7130);
  static const primarySoft = Color(0xFFC4EBCD);
  static const primaryTint = Color(0xFFEAF8EE);
  static const surface = Color(0xFFF7F7F7);
  static const card = Color(0xFFFFFFFF);
  static const text = Color(0xFF111111);
  static const textMuted = Color(0xFF747474);
  static const border = Color(0xFFE7E7E7);
  static const danger = Color(0xFFE53935);
  static const dangerSoft = Color(0xFFFFEBEE);
  static const warning = Color(0xFFF59E0B);
  static const warningSoft = Color(0xFFFFF7E6);
  static const info = Color(0xFF2563EB);
  static const infoSoft = Color(0xFFEAF2FF);

  const KarryGoColors._();
}

class KarryGoSpacing {
  static const xxs = 4.0;
  static const xs = 8.0;
  static const sm = 12.0;
  static const md = 16.0;
  static const lg = 24.0;
  static const xl = 32.0;
  static const xxl = 48.0;

  const KarryGoSpacing._();
}

class KarryGoTheme {
  const KarryGoTheme._();

  static ThemeData light() {
    final colorScheme = ColorScheme.fromSeed(
      seedColor: KarryGoColors.primary,
      error: KarryGoColors.danger,
      surface: KarryGoColors.surface,
    );

    return ThemeData(
      colorScheme: colorScheme,
      scaffoldBackgroundColor: KarryGoColors.surface,
      useMaterial3: true,
      appBarTheme: const AppBarTheme(
        backgroundColor: KarryGoColors.surface,
        foregroundColor: KarryGoColors.text,
        elevation: 0,
        centerTitle: false,
        titleTextStyle: TextStyle(
          color: KarryGoColors.text,
          fontSize: 17,
          fontWeight: FontWeight.w700,
        ),
      ),
      cardTheme: CardThemeData(
        color: KarryGoColors.card,
        elevation: 0,
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(20)),
      ),
      inputDecorationTheme: InputDecorationTheme(
        filled: true,
        fillColor: KarryGoColors.card,
        hintStyle: const TextStyle(color: KarryGoColors.textMuted),
        contentPadding: const EdgeInsets.symmetric(
          horizontal: KarryGoSpacing.md,
          vertical: KarryGoSpacing.sm,
        ),
        border: OutlineInputBorder(
          borderRadius: BorderRadius.circular(12),
          borderSide: BorderSide.none,
        ),
        enabledBorder: OutlineInputBorder(
          borderRadius: BorderRadius.circular(12),
          borderSide: BorderSide.none,
        ),
        focusedBorder: OutlineInputBorder(
          borderRadius: BorderRadius.circular(12),
          borderSide: const BorderSide(color: KarryGoColors.primary),
        ),
        errorBorder: OutlineInputBorder(
          borderRadius: BorderRadius.circular(12),
          borderSide: const BorderSide(color: KarryGoColors.danger),
        ),
      ),
      filledButtonTheme: FilledButtonThemeData(
        style: FilledButton.styleFrom(
          minimumSize: const Size.fromHeight(48),
          shape: RoundedRectangleBorder(
            borderRadius: BorderRadius.circular(24),
          ),
          textStyle: const TextStyle(fontWeight: FontWeight.w700, fontSize: 14),
        ),
      ),
      textTheme: const TextTheme(
        headlineSmall: TextStyle(
          color: KarryGoColors.text,
          fontSize: 24,
          fontWeight: FontWeight.w800,
          height: 1.15,
        ),
        titleLarge: TextStyle(
          color: KarryGoColors.text,
          fontSize: 20,
          fontWeight: FontWeight.w800,
        ),
        titleMedium: TextStyle(
          color: KarryGoColors.text,
          fontSize: 16,
          fontWeight: FontWeight.w700,
        ),
        bodyLarge: TextStyle(
          color: KarryGoColors.text,
          fontSize: 15,
          height: 1.45,
        ),
        bodyMedium: TextStyle(
          color: KarryGoColors.textMuted,
          fontSize: 13,
          height: 1.4,
        ),
        labelLarge: TextStyle(fontSize: 14, fontWeight: FontWeight.w700),
      ),
    );
  }
}

class KarryGoButton extends StatelessWidget {
  const KarryGoButton({
    super.key,
    required this.label,
    required this.onPressed,
    this.isLoading = false,
  });

  final String label;
  final VoidCallback? onPressed;
  final bool isLoading;

  @override
  Widget build(BuildContext context) {
    return SizedBox(
      width: double.infinity,
      height: 48,
      child: FilledButton(
        onPressed: isLoading ? null : onPressed,
        style: FilledButton.styleFrom(
          backgroundColor: KarryGoColors.primary,
          foregroundColor: Colors.white,
          disabledBackgroundColor: KarryGoColors.primarySoft,
          disabledForegroundColor: Colors.white,
        ),
        child: isLoading
            ? const SizedBox.square(
                dimension: 18,
                child: CircularProgressIndicator(
                  strokeWidth: 2,
                  valueColor: AlwaysStoppedAnimation<Color>(Colors.white),
                ),
              )
            : Text(label),
      ),
    );
  }
}

class KarryGoStepProgress extends StatelessWidget {
  const KarryGoStepProgress({
    super.key,
    required this.totalSteps,
    required this.currentStep,
  });

  final int totalSteps;
  final int currentStep;

  @override
  Widget build(BuildContext context) {
    return Row(
      children: List.generate(totalSteps, (index) {
        final isActive = index <= currentStep;
        return Expanded(
          child: Container(
            height: 4,
            margin: EdgeInsets.only(
              right: index == totalSteps - 1 ? 0 : KarryGoSpacing.xs,
            ),
            decoration: BoxDecoration(
              color: isActive
                  ? KarryGoColors.primary
                  : KarryGoColors.primarySoft,
              borderRadius: BorderRadius.circular(999),
            ),
          ),
        );
      }),
    );
  }
}

class KarryGoServiceOptionCard extends StatelessWidget {
  const KarryGoServiceOptionCard({
    super.key,
    required this.title,
    required this.subtitle,
    required this.icon,
    required this.onTap,
    this.isSelected = false,
  });

  final String title;
  final String subtitle;
  final Widget icon;
  final VoidCallback onTap;
  final bool isSelected;

  @override
  Widget build(BuildContext context) {
    return Material(
      color: isSelected ? KarryGoColors.primarySoft : KarryGoColors.card,
      borderRadius: BorderRadius.circular(20),
      child: InkWell(
        onTap: onTap,
        borderRadius: BorderRadius.circular(20),
        child: Container(
          constraints: const BoxConstraints(minHeight: 76),
          padding: const EdgeInsets.all(KarryGoSpacing.md),
          decoration: BoxDecoration(
            borderRadius: BorderRadius.circular(20),
            boxShadow: const [
              BoxShadow(
                color: Color(0x14000000),
                blurRadius: 20,
                offset: Offset(0, 8),
              ),
            ],
          ),
          child: Row(
            children: [
              SizedBox.square(dimension: 48, child: Center(child: icon)),
              const SizedBox(width: KarryGoSpacing.md),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    Text(title, style: Theme.of(context).textTheme.titleMedium),
                    const SizedBox(height: 2),
                    Text(
                      subtitle,
                      style: Theme.of(context).textTheme.bodyMedium,
                    ),
                  ],
                ),
              ),
              Icon(
                Icons.arrow_forward,
                size: 20,
                color: isSelected
                    ? KarryGoColors.primary
                    : KarryGoColors.border,
              ),
            ],
          ),
        ),
      ),
    );
  }
}

enum KarryGoFeedbackTone { error, warning, info, success }

extension KarryGoFeedbackToneColors on KarryGoFeedbackTone {
  Color get color {
    return switch (this) {
      KarryGoFeedbackTone.error => KarryGoColors.danger,
      KarryGoFeedbackTone.warning => KarryGoColors.warning,
      KarryGoFeedbackTone.info => KarryGoColors.info,
      KarryGoFeedbackTone.success => KarryGoColors.primary,
    };
  }

  Color get background {
    return switch (this) {
      KarryGoFeedbackTone.error => KarryGoColors.dangerSoft,
      KarryGoFeedbackTone.warning => KarryGoColors.warningSoft,
      KarryGoFeedbackTone.info => KarryGoColors.infoSoft,
      KarryGoFeedbackTone.success => KarryGoColors.primaryTint,
    };
  }

  IconData get icon {
    return switch (this) {
      KarryGoFeedbackTone.error => Icons.error_outline,
      KarryGoFeedbackTone.warning => Icons.warning_amber_rounded,
      KarryGoFeedbackTone.info => Icons.info_outline,
      KarryGoFeedbackTone.success => Icons.check_circle_outline,
    };
  }
}

class KarryGoErrorBanner extends StatelessWidget {
  const KarryGoErrorBanner({
    super.key,
    required this.message,
    this.title,
    this.tone = KarryGoFeedbackTone.error,
    this.onClose,
  });

  final String? title;
  final String message;
  final KarryGoFeedbackTone tone;
  final VoidCallback? onClose;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.all(KarryGoSpacing.md),
      decoration: BoxDecoration(
        color: tone.background,
        borderRadius: BorderRadius.circular(14),
        border: Border.all(color: tone.color.withValues(alpha: 0.22)),
      ),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Icon(tone.icon, color: tone.color, size: 20),
          const SizedBox(width: KarryGoSpacing.sm),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              mainAxisSize: MainAxisSize.min,
              children: [
                if (title != null) ...[
                  Text(
                    title!,
                    style: Theme.of(context).textTheme.titleMedium?.copyWith(
                      color: tone.color,
                      fontSize: 14,
                    ),
                  ),
                  const SizedBox(height: KarryGoSpacing.xxs),
                ],
                Text(
                  message,
                  style: Theme.of(
                    context,
                  ).textTheme.bodyMedium?.copyWith(color: KarryGoColors.text),
                ),
              ],
            ),
          ),
          if (onClose != null) ...[
            const SizedBox(width: KarryGoSpacing.xs),
            IconButton(
              onPressed: onClose,
              icon: const Icon(Icons.close),
              iconSize: 18,
              color: tone.color,
              visualDensity: VisualDensity.compact,
            ),
          ],
        ],
      ),
    );
  }
}

class KarryGoErrorView extends StatelessWidget {
  const KarryGoErrorView({
    super.key,
    required this.title,
    required this.message,
    this.actionLabel,
    this.onAction,
    this.tone = KarryGoFeedbackTone.error,
  });

  final String title;
  final String message;
  final String? actionLabel;
  final VoidCallback? onAction;
  final KarryGoFeedbackTone tone;

  @override
  Widget build(BuildContext context) {
    return Center(
      child: Padding(
        padding: const EdgeInsets.all(KarryGoSpacing.lg),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Container(
              width: 72,
              height: 72,
              decoration: BoxDecoration(
                color: tone.background,
                shape: BoxShape.circle,
              ),
              child: Icon(tone.icon, color: tone.color, size: 34),
            ),
            const SizedBox(height: KarryGoSpacing.lg),
            Text(
              title,
              textAlign: TextAlign.center,
              style: Theme.of(context).textTheme.titleLarge,
            ),
            const SizedBox(height: KarryGoSpacing.xs),
            Text(
              message,
              textAlign: TextAlign.center,
              style: Theme.of(context).textTheme.bodyMedium,
            ),
            if (actionLabel != null && onAction != null) ...[
              const SizedBox(height: KarryGoSpacing.lg),
              KarryGoButton(label: actionLabel!, onPressed: onAction),
            ],
          ],
        ),
      ),
    );
  }
}

class KarryGoFieldError extends StatelessWidget {
  const KarryGoFieldError({super.key, required this.message});

  final String message;

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.only(top: KarryGoSpacing.xs),
      child: Row(
        children: [
          const Icon(
            Icons.error_outline,
            size: 14,
            color: KarryGoColors.danger,
          ),
          const SizedBox(width: KarryGoSpacing.xxs),
          Expanded(
            child: Text(
              message,
              style: Theme.of(context).textTheme.bodyMedium?.copyWith(
                color: KarryGoColors.danger,
                fontSize: 12,
              ),
            ),
          ),
        ],
      ),
    );
  }
}

class KarryGoErrorSnackBar {
  const KarryGoErrorSnackBar._();

  static SnackBar build({
    required String message,
    KarryGoFeedbackTone tone = KarryGoFeedbackTone.error,
  }) {
    return SnackBar(
      behavior: SnackBarBehavior.floating,
      backgroundColor: tone.color,
      content: Row(
        children: [
          Icon(tone.icon, color: Colors.white, size: 18),
          const SizedBox(width: KarryGoSpacing.sm),
          Expanded(child: Text(message)),
        ],
      ),
    );
  }

  static void show(
    BuildContext context, {
    required String message,
    KarryGoFeedbackTone tone = KarryGoFeedbackTone.error,
  }) {
    ScaffoldMessenger.of(
      context,
    ).showSnackBar(build(message: message, tone: tone));
  }
}

class KarryGoOtpInput extends StatelessWidget {
  const KarryGoOtpInput({
    super.key,
    required this.length,
    required this.value,
    this.hasError = false,
  });

  final int length;
  final String value;
  final bool hasError;

  @override
  Widget build(BuildContext context) {
    final chars = value.split('').take(length).toList();

    return Row(
      mainAxisAlignment: MainAxisAlignment.spaceBetween,
      children: List.generate(length, (index) {
        final hasValue = index < chars.length;
        final color = hasError ? KarryGoColors.danger : KarryGoColors.primary;

        return Container(
          width: 42,
          height: 42,
          alignment: Alignment.center,
          decoration: BoxDecoration(
            color: hasValue ? Colors.white : Colors.white,
            shape: BoxShape.circle,
            border: Border.all(
              color: hasValue || hasError ? color : Colors.transparent,
            ),
            boxShadow: const [
              BoxShadow(
                color: Color(0x12000000),
                blurRadius: 16,
                offset: Offset(0, 6),
              ),
            ],
          ),
          child: Text(
            hasValue ? chars[index] : '',
            style: Theme.of(context).textTheme.titleMedium,
          ),
        );
      }),
    );
  }
}
