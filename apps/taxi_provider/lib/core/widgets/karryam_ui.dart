import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:flutter/material.dart';

import '../theme/karryam_theme.dart';

/// A pushed screen: back button, title and an optional line under it.
class KarryamPage extends StatelessWidget {
  const KarryamPage({
    super.key,
    required this.title,
    required this.body,
    this.subtitle,
    this.actions,
    this.bottom,
  });

  final String title;
  final String? subtitle;
  final Widget body;
  final List<Widget>? actions;

  /// Pinned under the body (e.g. a primary button), above the gesture area.
  final Widget? bottom;

  @override
  Widget build(BuildContext context) {
    final colors = KarryamColors.of(context);
    return Scaffold(
      backgroundColor: colors.surface,
      appBar: AppBar(
        backgroundColor: colors.surface,
        surfaceTintColor: Colors.transparent,
        title: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(
              title,
              style: const TextStyle(fontSize: 18, fontWeight: FontWeight.w700),
            ),
            if (subtitle != null)
              Text(
                subtitle!,
                style: TextStyle(fontSize: 12, color: colors.textMuted),
              ),
          ],
        ),
        actions: actions,
      ),
      body: SafeArea(top: false, child: body),
      bottomNavigationBar: bottom == null
          ? null
          : SafeArea(
              child: Padding(
                padding: const EdgeInsets.fromLTRB(20, 8, 20, 12),
                child: bottom,
              ),
            ),
    );
  }
}

/// White (or dark-card) rounded panel.
class KarryamPanel extends StatelessWidget {
  const KarryamPanel({
    super.key,
    required this.child,
    this.padding,
    this.onTap,
  });

  final Widget child;
  final EdgeInsetsGeometry? padding;
  final VoidCallback? onTap;

  @override
  Widget build(BuildContext context) {
    final colors = KarryamColors.of(context);
    return Material(
      color: colors.card,
      borderRadius: BorderRadius.circular(16),
      child: InkWell(
        borderRadius: BorderRadius.circular(16),
        onTap: onTap,
        child: Container(
          width: double.infinity,
          padding: padding ?? const EdgeInsets.all(16),
          decoration: BoxDecoration(
            borderRadius: BorderRadius.circular(16),
            border: Border.all(color: colors.border),
          ),
          child: child,
        ),
      ),
    );
  }
}

class KarryamSectionTitle extends StatelessWidget {
  const KarryamSectionTitle(this.text, {super.key});
  final String text;

  @override
  Widget build(BuildContext context) => Padding(
    padding: const EdgeInsets.only(top: 24, bottom: 12),
    child: Text(
      text,
      style: const TextStyle(fontSize: 16, fontWeight: FontWeight.w700),
    ),
  );
}

/// The app's primary action button, with a spinner while [busy].
class KarryamButton extends StatelessWidget {
  const KarryamButton({
    super.key,
    required this.label,
    required this.onPressed,
    this.busy = false,
    this.danger = false,
  });

  final String label;
  final VoidCallback? onPressed;
  final bool busy;
  final bool danger;

  @override
  Widget build(BuildContext context) {
    final colors = KarryamColors.of(context);
    final background = danger ? colors.danger : colors.primary;
    final foreground = danger ? Colors.white : colors.onPrimary;
    return SizedBox(
      height: 52,
      width: double.infinity,
      child: FilledButton(
        onPressed: busy ? null : onPressed,
        style: FilledButton.styleFrom(
          backgroundColor: background,
          disabledBackgroundColor: colors.primarySoft,
          shape: RoundedRectangleBorder(
            borderRadius: BorderRadius.circular(12),
          ),
        ),
        child: busy
            ? SizedBox(
                width: 20,
                height: 20,
                child: CircularProgressIndicator(
                  strokeWidth: 2,
                  color: foreground,
                ),
              )
            : Text(
                label,
                style: TextStyle(
                  color: foreground,
                  fontSize: 16,
                  fontWeight: FontWeight.w700,
                ),
              ),
      ),
    );
  }
}

/// Text field in the app's style.
class KarryamField extends StatelessWidget {
  const KarryamField({
    super.key,
    required this.controller,
    required this.label,
    this.hint,
    this.keyboardType,
    this.maxLines = 1,
    this.readOnly = false,
    this.onTap,
    this.suffix,
    this.textCapitalization = TextCapitalization.none,
  });

  final TextEditingController controller;
  final String label;
  final String? hint;
  final TextInputType? keyboardType;
  final int maxLines;
  final bool readOnly;
  final VoidCallback? onTap;
  final Widget? suffix;
  final TextCapitalization textCapitalization;

  @override
  Widget build(BuildContext context) {
    final colors = KarryamColors.of(context);
    final border = OutlineInputBorder(
      borderRadius: BorderRadius.circular(12),
      borderSide: BorderSide(color: colors.border),
    );
    return Padding(
      padding: const EdgeInsets.only(bottom: 14),
      child: TextField(
        controller: controller,
        keyboardType: keyboardType,
        maxLines: maxLines,
        readOnly: readOnly,
        onTap: onTap,
        textCapitalization: textCapitalization,
        decoration: InputDecoration(
          labelText: label,
          hintText: hint,
          filled: true,
          fillColor: colors.card,
          suffixIcon: suffix,
          border: border,
          enabledBorder: border,
        ),
      ),
    );
  }
}

/// Loading, error-with-retry or empty placeholder for a list screen.
class KarryamPlaceholder extends StatelessWidget {
  const KarryamPlaceholder.loading({super.key})
    : icon = null,
      message = null,
      onRetry = null,
      loading = true;
  const KarryamPlaceholder.empty({
    super.key,
    required IconData this.icon,
    required String this.message,
  }) : onRetry = null,
       loading = false;
  const KarryamPlaceholder.error({
    super.key,
    required String this.message,
    required this.onRetry,
  }) : icon = Icons.cloud_off_rounded,
       loading = false;

  final IconData? icon;
  final String? message;
  final VoidCallback? onRetry;
  final bool loading;

  @override
  Widget build(BuildContext context) {
    final colors = KarryamColors.of(context);
    if (loading) {
      return const Center(
        child: Padding(
          padding: EdgeInsets.all(32),
          child: CircularProgressIndicator(),
        ),
      );
    }
    return Center(
      child: Padding(
        padding: const EdgeInsets.all(32),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(icon, size: 40, color: colors.textMuted),
            const SizedBox(height: 12),
            Text(
              message!,
              textAlign: TextAlign.center,
              style: TextStyle(color: colors.textMuted),
            ),
            if (onRetry != null) ...[
              const SizedBox(height: 12),
              TextButton(onPressed: onRetry, child: const Text('Try again')),
            ],
          ],
        ),
      ),
    );
  }
}

/// Small rounded status label.
class KarryamChip extends StatelessWidget {
  const KarryamChip(this.label, {super.key, this.tone = KarryamTone.neutral});
  final String label;
  final KarryamTone tone;

  @override
  Widget build(BuildContext context) {
    final colors = KarryamColors.of(context);
    final (bg, fg) = switch (tone) {
      KarryamTone.good => (colors.successSoft, colors.text),
      KarryamTone.warning => (colors.warningSoft, colors.warning),
      KarryamTone.bad => (colors.dangerSoft, colors.danger),
      KarryamTone.neutral => (colors.primaryTint, colors.text),
    };
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 4),
      decoration: BoxDecoration(
        color: bg,
        borderRadius: BorderRadius.circular(20),
      ),
      child: Text(
        label,
        style: TextStyle(color: fg, fontSize: 12, fontWeight: FontWeight.w600),
      ),
    );
  }
}

enum KarryamTone { good, warning, bad, neutral }

/// Shows an API failure as a snackbar.
void showApiError(BuildContext context, Object error) {
  final message = error is ApiException
      ? error.message
      : 'Something went wrong. Please try again.';
  ScaffoldMessenger.of(context)
    ..hideCurrentSnackBar()
    ..showSnackBar(SnackBar(content: Text(message)));
}

/// "18 Oct 2026, 3:05 PM".
String formatDateTime(DateTime? time) {
  if (time == null) return '';
  const months = [
    'Jan',
    'Feb',
    'Mar',
    'Apr',
    'May',
    'Jun',
    'Jul',
    'Aug',
    'Sep',
    'Oct',
    'Nov',
    'Dec',
  ];
  final hour = time.hour % 12 == 0 ? 12 : time.hour % 12;
  final minute = time.minute.toString().padLeft(2, '0');
  final period = time.hour < 12 ? 'AM' : 'PM';
  return '${time.day} ${months[time.month - 1]} ${time.year}, $hour:$minute $period';
}

/// "pending_review" -> "Pending review".
String humanize(String value) {
  if (value.isEmpty) return 'Not started';
  final words = value.replaceAll('_', ' ').trim();
  return words[0].toUpperCase() + words.substring(1);
}
