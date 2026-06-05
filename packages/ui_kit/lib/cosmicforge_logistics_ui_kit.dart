library;

import 'package:flutter/material.dart';

class CosmicforgeLogisticsColors {
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

  const CosmicforgeLogisticsColors._();
}

class CosmicforgeLogisticsSpacing {
  static const xxs = 4.0;
  static const xs = 8.0;
  static const sm = 12.0;
  static const md = 16.0;
  static const lg = 24.0;
  static const xl = 32.0;
  static const xxl = 48.0;

  const CosmicforgeLogisticsSpacing._();
}

class CosmicforgeLogisticsTheme {
  const CosmicforgeLogisticsTheme._();

  static ThemeData light() {
    final colorScheme = ColorScheme.fromSeed(
      seedColor: CosmicforgeLogisticsColors.primary,
      error: CosmicforgeLogisticsColors.danger,
      surface: CosmicforgeLogisticsColors.surface,
    );

    return ThemeData(
      colorScheme: colorScheme,
      scaffoldBackgroundColor: CosmicforgeLogisticsColors.surface,
      useMaterial3: true,
      appBarTheme: const AppBarTheme(
        backgroundColor: CosmicforgeLogisticsColors.surface,
        foregroundColor: CosmicforgeLogisticsColors.text,
        elevation: 0,
        centerTitle: false,
        titleTextStyle: TextStyle(
          color: CosmicforgeLogisticsColors.text,
          fontSize: 17,
          fontWeight: FontWeight.w700,
        ),
      ),
      cardTheme: CardThemeData(
        color: CosmicforgeLogisticsColors.card,
        elevation: 0,
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(20)),
      ),
      inputDecorationTheme: InputDecorationTheme(
        filled: true,
        fillColor: CosmicforgeLogisticsColors.card,
        hintStyle: const TextStyle(color: CosmicforgeLogisticsColors.textMuted),
        contentPadding: const EdgeInsets.symmetric(
          horizontal: CosmicforgeLogisticsSpacing.md,
          vertical: CosmicforgeLogisticsSpacing.sm,
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
          borderSide: const BorderSide(color: CosmicforgeLogisticsColors.primary),
        ),
        errorBorder: OutlineInputBorder(
          borderRadius: BorderRadius.circular(12),
          borderSide: const BorderSide(color: CosmicforgeLogisticsColors.danger),
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
          color: CosmicforgeLogisticsColors.text,
          fontSize: 24,
          fontWeight: FontWeight.w800,
          height: 1.15,
        ),
        titleLarge: TextStyle(
          color: CosmicforgeLogisticsColors.text,
          fontSize: 20,
          fontWeight: FontWeight.w800,
        ),
        titleMedium: TextStyle(
          color: CosmicforgeLogisticsColors.text,
          fontSize: 16,
          fontWeight: FontWeight.w700,
        ),
        bodyLarge: TextStyle(
          color: CosmicforgeLogisticsColors.text,
          fontSize: 15,
          height: 1.45,
        ),
        bodyMedium: TextStyle(
          color: CosmicforgeLogisticsColors.textMuted,
          fontSize: 13,
          height: 1.4,
        ),
        labelLarge: TextStyle(fontSize: 14, fontWeight: FontWeight.w700),
      ),
    );
  }
}

class CosmicforgeLogisticsButton extends StatelessWidget {
  const CosmicforgeLogisticsButton({
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
          backgroundColor: CosmicforgeLogisticsColors.primary,
          foregroundColor: Colors.white,
          disabledBackgroundColor: CosmicforgeLogisticsColors.primarySoft,
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

class CosmicforgeLogisticsStepProgress extends StatelessWidget {
  const CosmicforgeLogisticsStepProgress({
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
              right: index == totalSteps - 1 ? 0 : CosmicforgeLogisticsSpacing.xs,
            ),
            decoration: BoxDecoration(
              color: isActive
                  ? CosmicforgeLogisticsColors.primary
                  : CosmicforgeLogisticsColors.primarySoft,
              borderRadius: BorderRadius.circular(999),
            ),
          ),
        );
      }),
    );
  }
}

class CosmicforgeLogisticsServiceOptionCard extends StatelessWidget {
  const CosmicforgeLogisticsServiceOptionCard({
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
      color: isSelected ? CosmicforgeLogisticsColors.primarySoft : CosmicforgeLogisticsColors.card,
      borderRadius: BorderRadius.circular(20),
      child: InkWell(
        onTap: onTap,
        borderRadius: BorderRadius.circular(20),
        child: Container(
          constraints: const BoxConstraints(minHeight: 76),
          padding: const EdgeInsets.all(CosmicforgeLogisticsSpacing.md),
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
              const SizedBox(width: CosmicforgeLogisticsSpacing.md),
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
                    ? CosmicforgeLogisticsColors.primary
                    : CosmicforgeLogisticsColors.border,
              ),
            ],
          ),
        ),
      ),
    );
  }
}

enum CosmicforgeLogisticsFeedbackTone { error, warning, info, success }

extension CosmicforgeLogisticsFeedbackToneColors on CosmicforgeLogisticsFeedbackTone {
  Color get color {
    return switch (this) {
      CosmicforgeLogisticsFeedbackTone.error => CosmicforgeLogisticsColors.danger,
      CosmicforgeLogisticsFeedbackTone.warning => CosmicforgeLogisticsColors.warning,
      CosmicforgeLogisticsFeedbackTone.info => CosmicforgeLogisticsColors.info,
      CosmicforgeLogisticsFeedbackTone.success => CosmicforgeLogisticsColors.primary,
    };
  }

  Color get background {
    return switch (this) {
      CosmicforgeLogisticsFeedbackTone.error => CosmicforgeLogisticsColors.dangerSoft,
      CosmicforgeLogisticsFeedbackTone.warning => CosmicforgeLogisticsColors.warningSoft,
      CosmicforgeLogisticsFeedbackTone.info => CosmicforgeLogisticsColors.infoSoft,
      CosmicforgeLogisticsFeedbackTone.success => CosmicforgeLogisticsColors.primaryTint,
    };
  }

  IconData get icon {
    return switch (this) {
      CosmicforgeLogisticsFeedbackTone.error => Icons.error_outline,
      CosmicforgeLogisticsFeedbackTone.warning => Icons.warning_amber_rounded,
      CosmicforgeLogisticsFeedbackTone.info => Icons.info_outline,
      CosmicforgeLogisticsFeedbackTone.success => Icons.check_circle_outline,
    };
  }
}

class CosmicforgeLogisticsErrorBanner extends StatelessWidget {
  const CosmicforgeLogisticsErrorBanner({
    super.key,
    required this.message,
    this.title,
    this.tone = CosmicforgeLogisticsFeedbackTone.error,
    this.onClose,
  });

  final String? title;
  final String message;
  final CosmicforgeLogisticsFeedbackTone tone;
  final VoidCallback? onClose;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.all(CosmicforgeLogisticsSpacing.md),
      decoration: BoxDecoration(
        color: tone.background,
        borderRadius: BorderRadius.circular(14),
        border: Border.all(color: tone.color.withValues(alpha: 0.22)),
      ),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Icon(tone.icon, color: tone.color, size: 20),
          const SizedBox(width: CosmicforgeLogisticsSpacing.sm),
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
                  const SizedBox(height: CosmicforgeLogisticsSpacing.xxs),
                ],
                Text(
                  message,
                  style: Theme.of(
                    context,
                  ).textTheme.bodyMedium?.copyWith(color: CosmicforgeLogisticsColors.text),
                ),
              ],
            ),
          ),
          if (onClose != null) ...[
            const SizedBox(width: CosmicforgeLogisticsSpacing.xs),
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

class CosmicforgeLogisticsErrorView extends StatelessWidget {
  const CosmicforgeLogisticsErrorView({
    super.key,
    required this.title,
    required this.message,
    this.actionLabel,
    this.onAction,
    this.tone = CosmicforgeLogisticsFeedbackTone.error,
  });

  final String title;
  final String message;
  final String? actionLabel;
  final VoidCallback? onAction;
  final CosmicforgeLogisticsFeedbackTone tone;

  @override
  Widget build(BuildContext context) {
    return Center(
      child: Padding(
        padding: const EdgeInsets.all(CosmicforgeLogisticsSpacing.lg),
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
            const SizedBox(height: CosmicforgeLogisticsSpacing.lg),
            Text(
              title,
              textAlign: TextAlign.center,
              style: Theme.of(context).textTheme.titleLarge,
            ),
            const SizedBox(height: CosmicforgeLogisticsSpacing.xs),
            Text(
              message,
              textAlign: TextAlign.center,
              style: Theme.of(context).textTheme.bodyMedium,
            ),
            if (actionLabel != null && onAction != null) ...[
              const SizedBox(height: CosmicforgeLogisticsSpacing.lg),
              CosmicforgeLogisticsButton(label: actionLabel!, onPressed: onAction),
            ],
          ],
        ),
      ),
    );
  }
}

class CosmicforgeLogisticsFieldError extends StatelessWidget {
  const CosmicforgeLogisticsFieldError({super.key, required this.message});

  final String message;

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.only(top: CosmicforgeLogisticsSpacing.xs),
      child: Row(
        children: [
          const Icon(
            Icons.error_outline,
            size: 14,
            color: CosmicforgeLogisticsColors.danger,
          ),
          const SizedBox(width: CosmicforgeLogisticsSpacing.xxs),
          Expanded(
            child: Text(
              message,
              style: Theme.of(context).textTheme.bodyMedium?.copyWith(
                color: CosmicforgeLogisticsColors.danger,
                fontSize: 12,
              ),
            ),
          ),
        ],
      ),
    );
  }
}

class CosmicforgeLogisticsErrorSnackBar {
  const CosmicforgeLogisticsErrorSnackBar._();

  static SnackBar build({
    required String message,
    CosmicforgeLogisticsFeedbackTone tone = CosmicforgeLogisticsFeedbackTone.error,
  }) {
    return SnackBar(
      behavior: SnackBarBehavior.floating,
      backgroundColor: tone.color,
      content: Row(
        children: [
          Icon(tone.icon, color: Colors.white, size: 18),
          const SizedBox(width: CosmicforgeLogisticsSpacing.sm),
          Expanded(child: Text(message)),
        ],
      ),
    );
  }

  static void show(
    BuildContext context, {
    required String message,
    CosmicforgeLogisticsFeedbackTone tone = CosmicforgeLogisticsFeedbackTone.error,
  }) {
    ScaffoldMessenger.of(
      context,
    ).showSnackBar(build(message: message, tone: tone));
  }
}

class CosmicforgeLogisticsOtpInput extends StatelessWidget {
  const CosmicforgeLogisticsOtpInput({
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
        final color = hasError ? CosmicforgeLogisticsColors.danger : CosmicforgeLogisticsColors.primary;

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
