import 'package:flutter/material.dart';

import '../../features/home/ui/widgets/provider_app_colors.dart';

/// Shared Karryam building blocks for the truck partner app.
///
/// Every screen composes these instead of hand-rolling its own header, card or
/// button, so spacing, corners and colours stay identical across the app:
/// 20px page insets, 8px rhythm, 18px cards, 52px primary actions.

const kPageInset = EdgeInsets.symmetric(horizontal: 20);
const kCardRadius = BorderRadius.all(Radius.circular(18));

// ─── Top bar ─────────────────────────────────────────────────────────────────

/// Back circle, centred title, optional trailing action.
class KTopBar extends StatelessWidget implements PreferredSizeWidget {
  KTopBar({
    super.key,
    this.title,
    this.onBack,
    this.trailing,
    this.showBack = true,
    Color? background,
  }) : background = background ?? kCanvas;

  final String? title;
  final VoidCallback? onBack;
  final Widget? trailing;
  final bool showBack;
  final Color background;

  @override
  Size get preferredSize => const Size.fromHeight(64);

  @override
  Widget build(BuildContext context) {
    final canPop = Navigator.of(context).canPop();
    return Material(
      color: background,
      child: SafeArea(
        bottom: false,
        child: SizedBox(
          height: 64,
          child: Padding(
            padding: const EdgeInsets.symmetric(horizontal: 16),
            child: Row(
              children: [
                SizedBox(
                  width: 44,
                  child: showBack && (onBack != null || canPop)
                      ? KCircleButton(
                          icon: Icons.arrow_back_rounded,
                          tooltip: 'Back',
                          onTap: onBack ?? () => Navigator.of(context).maybePop(),
                        )
                      : null,
                ),
                Expanded(
                  child: Text(
                    title ?? '',
                    textAlign: TextAlign.center,
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis,
                    style: TextStyle(
                      fontSize: 15,
                      fontWeight: FontWeight.w600,
                      color: kProviderText,
                    ),
                  ),
                ),
                SizedBox(width: 44, child: trailing),
              ],
            ),
          ),
        ),
      ),
    );
  }
}

/// A 44px round white button with a hairline border.
class KCircleButton extends StatelessWidget {
  KCircleButton({
    super.key,
    required this.icon,
    required this.onTap,
    this.tooltip,
    this.badge = false,
    Color? background,
  }) : background = background ?? kCard;

  final IconData icon;
  final VoidCallback? onTap;
  final String? tooltip;
  final bool badge;
  final Color background;

  @override
  Widget build(BuildContext context) {
    final button = Material(
      color: background,
      shape: CircleBorder(side: BorderSide(color: kProviderBorder)),
      child: InkWell(
        customBorder: const CircleBorder(),
        onTap: onTap,
        child: SizedBox(
          width: 44,
          height: 44,
          child: Stack(
            alignment: Alignment.center,
            children: [
              Icon(icon, size: 20, color: kProviderText),
              if (badge)
                Positioned(
                  top: 10,
                  right: 11,
                  child: Container(
                    width: 8,
                    height: 8,
                    decoration: BoxDecoration(
                      color: kTeal,
                      shape: BoxShape.circle,
                      border: Border.all(color: background, width: 1.5),
                    ),
                  ),
                ),
            ],
          ),
        ),
      ),
    );
    return tooltip == null ? button : Tooltip(message: tooltip!, child: button);
  }
}

// ─── Headings ────────────────────────────────────────────────────────────────

/// Small-caps label above a heading: "FIND YOUR NEXT MOVE".
class KEyebrow extends StatelessWidget {
  KEyebrow(this.text, {super.key, Color? color}) : color = color ?? kProviderMuted;
  final String text;
  final Color color;

  @override
  Widget build(BuildContext context) => Text(
        text.toUpperCase(),
        style: TextStyle(
          fontSize: 11,
          letterSpacing: 1.4,
          fontWeight: FontWeight.w600,
          color: color,
        ),
      );
}

/// Eyebrow, large heading and muted subtitle — the opening of most screens.
class KHeading extends StatelessWidget {
  const KHeading({
    super.key,
    required this.title,
    this.eyebrow,
    this.subtitle,
    this.trailing,
    this.center = false,
  });

  final String title;
  final String? eyebrow;
  final String? subtitle;
  final Widget? trailing;
  final bool center;

  @override
  Widget build(BuildContext context) {
    final align = center ? CrossAxisAlignment.center : CrossAxisAlignment.start;
    final textAlign = center ? TextAlign.center : TextAlign.start;
    final column = Column(
      crossAxisAlignment: align,
      children: [
        if (eyebrow != null) ...[KEyebrow(eyebrow!), const SizedBox(height: 8)],
        Text(
          title,
          textAlign: textAlign,
          style: TextStyle(
            fontSize: 26,
            height: 1.15,
            fontWeight: FontWeight.w700,
            letterSpacing: -0.6,
            color: kProviderText,
          ),
        ),
        if (subtitle != null) ...[
          const SizedBox(height: 8),
          Text(
            subtitle!,
            textAlign: textAlign,
            style: TextStyle(fontSize: 14, height: 1.45, color: kProviderMuted),
          ),
        ],
      ],
    );
    if (trailing == null) return column;
    return Row(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [Expanded(child: column), const SizedBox(width: 12), trailing!],
    );
  }
}

/// Section title with an optional text action on the right.
class KSectionHeader extends StatelessWidget {
  const KSectionHeader(this.title, {super.key, this.action, this.onAction});
  final String title;
  final String? action;
  final VoidCallback? onAction;

  @override
  Widget build(BuildContext context) {
    return Row(
      children: [
        Expanded(
          child: Text(
            title,
            style: TextStyle(fontSize: 15, fontWeight: FontWeight.w700, color: kProviderText),
          ),
        ),
        if (action != null)
          InkWell(
            onTap: onAction,
            borderRadius: BorderRadius.circular(8),
            child: Padding(
              padding: const EdgeInsets.symmetric(horizontal: 4, vertical: 4),
              child: Row(
                mainAxisSize: MainAxisSize.min,
                children: [
                  Text(
                    action!,
                    style: TextStyle(fontSize: 13, fontWeight: FontWeight.w600, color: kTealText),
                  ),
                  const SizedBox(width: 2),
                  Icon(Icons.north_east_rounded, size: 14, color: kTealText),
                ],
              ),
            ),
          ),
      ],
    );
  }
}

// ─── Buttons ─────────────────────────────────────────────────────────────────

/// The navy 52px primary action. Shows a spinner while [loading].
class KPrimaryButton extends StatelessWidget {
  KPrimaryButton({
    super.key,
    required this.label,
    required this.onPressed,
    this.loading = false,
    this.arrow = false,
    this.icon,
    Color? color,
    Color? foreground,
  }) : color = color ?? kPrimary, foreground = foreground ?? kOnPrimary;

  final String label;
  final VoidCallback? onPressed;
  final bool loading;
  final bool arrow;
  final IconData? icon;
  final Color color;
  final Color foreground;

  @override
  Widget build(BuildContext context) {
    return SizedBox(
      width: double.infinity,
      height: 52,
      child: FilledButton(
        onPressed: loading ? null : onPressed,
        style: FilledButton.styleFrom(
          backgroundColor: color,
          foregroundColor: foreground,
          disabledBackgroundColor: loading ? color : kProviderGreenSoft,
          disabledForegroundColor: foreground,
        ),
        child: loading
            ? SizedBox(
                width: 22,
                height: 22,
                child: CircularProgressIndicator(strokeWidth: 2.4, color: foreground),
              )
            : Row(
                mainAxisSize: MainAxisSize.min,
                children: [
                  if (icon != null) ...[Icon(icon, size: 20), const SizedBox(width: 8)],
                  Flexible(child: Text(label, overflow: TextOverflow.ellipsis)),
                  if (arrow) ...[const SizedBox(width: 8), const Icon(Icons.arrow_forward_rounded, size: 18)],
                ],
              ),
      ),
    );
  }
}

/// White outlined 52px secondary action. [destructive] turns the label red.
class KSecondaryButton extends StatelessWidget {
  const KSecondaryButton({
    super.key,
    required this.label,
    required this.onPressed,
    this.icon,
    this.destructive = false,
    this.loading = false,
  });

  final String label;
  final VoidCallback? onPressed;
  final IconData? icon;
  final bool destructive;
  final bool loading;

  @override
  Widget build(BuildContext context) {
    final fg = destructive ? kProviderRejectText : kProviderText;
    return SizedBox(
      width: double.infinity,
      height: 52,
      child: OutlinedButton(
        onPressed: loading ? null : onPressed,
        style: OutlinedButton.styleFrom(foregroundColor: fg),
        child: loading
            ? SizedBox(width: 20, height: 20, child: CircularProgressIndicator(strokeWidth: 2.2, color: fg))
            : Row(
                mainAxisSize: MainAxisSize.min,
                children: [
                  if (icon != null) ...[Icon(icon, size: 19), const SizedBox(width: 8)],
                  Flexible(child: Text(label, overflow: TextOverflow.ellipsis)),
                ],
              ),
      ),
    );
  }
}

// ─── Surfaces ────────────────────────────────────────────────────────────────

/// White rounded card with a hairline border.
class KCard extends StatelessWidget {
  KCard({
    super.key,
    required this.child,
    this.padding = const EdgeInsets.all(16),
    this.onTap,
    Color? color,
    Color? borderColor,
  }) : color = color ?? kCard, borderColor = borderColor ?? kProviderBorder;

  final Widget child;
  final EdgeInsetsGeometry padding;
  final VoidCallback? onTap;
  final Color color;
  final Color borderColor;

  @override
  Widget build(BuildContext context) {
    return Material(
      color: color,
      shape: RoundedRectangleBorder(
        borderRadius: kCardRadius,
        side: BorderSide(color: borderColor),
      ),
      clipBehavior: Clip.antiAlias,
      child: InkWell(
        onTap: onTap,
        child: Padding(padding: padding, child: child),
      ),
    );
  }
}

/// Soft tinted square holding an icon — the leading element of list rows.
class KIconTile extends StatelessWidget {
  KIconTile(
    this.icon, {
    super.key,
    Color? color,
    Color? background,
    this.size = 40,
  }) : color = color ?? kTealText, background = background ?? kTealTint;

  final IconData icon;
  final Color color;
  final Color background;
  final double size;

  @override
  Widget build(BuildContext context) => Container(
        width: size,
        height: size,
        decoration: BoxDecoration(
          color: background,
          borderRadius: BorderRadius.circular(size * 0.3),
        ),
        child: Icon(icon, color: color, size: size * 0.5),
      );
}

/// Tinted icon tile, title, optional subtitle, trailing chevron.
class KListRow extends StatelessWidget {
  KListRow({
    super.key,
    required this.title,
    this.subtitle,
    this.icon,
    this.leading,
    this.trailing,
    this.onTap,
    Color? iconColor,
    Color? iconBackground,
    Color? titleColor,
    this.showChevron = true,
    this.dense = false,
  }) : iconColor = iconColor ?? kTealText, iconBackground = iconBackground ?? kTealTint, titleColor = titleColor ?? kProviderText;

  final String title;
  final String? subtitle;
  final IconData? icon;
  final Widget? leading;
  final Widget? trailing;
  final VoidCallback? onTap;
  final Color iconColor;
  final Color iconBackground;
  final Color titleColor;
  final bool showChevron;
  final bool dense;

  @override
  Widget build(BuildContext context) {
    final lead = leading ?? (icon == null ? null : KIconTile(icon!, color: iconColor, background: iconBackground));
    return InkWell(
      onTap: onTap,
      child: Padding(
        padding: EdgeInsets.symmetric(horizontal: 16, vertical: dense ? 10 : 14),
        child: Row(
          children: [
            if (lead != null) ...[lead, const SizedBox(width: 14)],
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    title,
                    style: TextStyle(fontSize: 14, fontWeight: FontWeight.w600, color: titleColor),
                  ),
                  if (subtitle != null && subtitle!.isNotEmpty) ...[
                    const SizedBox(height: 3),
                    Text(
                      subtitle!,
                      maxLines: 2,
                      overflow: TextOverflow.ellipsis,
                      style: TextStyle(fontSize: 12.5, height: 1.35, color: kProviderMuted),
                    ),
                  ],
                ],
              ),
            ),
            if (trailing != null) ...[const SizedBox(width: 10), trailing!],
            if (showChevron && onTap != null) ...[
              const SizedBox(width: 6),
              Icon(Icons.chevron_right_rounded, color: kProviderMuted, size: 22),
            ],
          ],
        ),
      ),
    );
  }
}

/// A card holding several [KListRow]s separated by inset dividers.
class KGroup extends StatelessWidget {
  const KGroup({super.key, required this.children});
  final List<Widget> children;

  @override
  Widget build(BuildContext context) {
    final items = <Widget>[];
    for (var i = 0; i < children.length; i++) {
      if (i > 0) items.add(const Divider(indent: 16, endIndent: 16));
      items.add(children[i]);
    }
    return KCard(padding: EdgeInsets.zero, child: Column(children: items));
  }
}

// ─── Status ──────────────────────────────────────────────────────────────────

enum KTone { success, info, warning, danger, neutral }

/// Rounded pill with a leading dot: "● Active delivery".
class KStatusChip extends StatelessWidget {
  const KStatusChip(this.label, {super.key, this.tone = KTone.success, this.dot = true});
  final String label;
  final KTone tone;
  final bool dot;

  (Color, Color) get _colors => switch (tone) {
        KTone.success => (kTealText, kTealTint),
        KTone.info => (kProviderText, kProviderGreenTint),
        KTone.warning => (kWarning, kWarningTint),
        KTone.danger => (kProviderRejectText, kProviderRejectBg),
        KTone.neutral => (kProviderMuted, kProviderGreenTint),
      };

  @override
  Widget build(BuildContext context) {
    final (fg, bg) = _colors;
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 5),
      decoration: BoxDecoration(color: bg, borderRadius: BorderRadius.circular(999)),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          if (dot) ...[
            Container(width: 6, height: 6, decoration: BoxDecoration(color: fg, shape: BoxShape.circle)),
            const SizedBox(width: 6),
          ],
          Text(label, style: TextStyle(fontSize: 11.5, fontWeight: FontWeight.w600, color: fg)),
        ],
      ),
    );
  }
}

// ─── Route ───────────────────────────────────────────────────────────────────

/// Pickup (ring) and delivery (square) stops joined by a dashed line.
class KRouteStops extends StatelessWidget {
  const KRouteStops({
    super.key,
    required this.pickup,
    required this.dropoff,
    this.pickupLabel = 'Pickup',
    this.dropoffLabel = 'Delivery',
    this.maxLines = 2,
  });

  final String pickup;
  final String dropoff;
  final String pickupLabel;
  final String dropoffLabel;

  /// Lines per address; null shows addresses in full.
  final int? maxLines;

  @override
  Widget build(BuildContext context) {
    return IntrinsicHeight(
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          SizedBox(
            width: 18,
            child: Column(
              children: [
                const SizedBox(height: 3),
                Container(
                  width: 14,
                  height: 14,
                  decoration: BoxDecoration(
                    shape: BoxShape.circle,
                    border: Border.all(color: kTeal, width: 3),
                  ),
                ),
                Expanded(child: CustomPaint(painter: _DashPainter(), child: const SizedBox(width: 2))),
                Container(
                  width: 12,
                  height: 12,
                  decoration: BoxDecoration(
                    color: kCard,
                    border: Border.all(color: kProviderText, width: 2.5),
                    borderRadius: BorderRadius.circular(2),
                  ),
                ),
                const SizedBox(height: 4),
              ],
            ),
          ),
          const SizedBox(width: 12),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                _Stop(label: pickupLabel, address: pickup, maxLines: maxLines),
                const SizedBox(height: 18),
                _Stop(label: dropoffLabel, address: dropoff, maxLines: maxLines),
              ],
            ),
          ),
        ],
      ),
    );
  }
}

class _Stop extends StatelessWidget {
  const _Stop({required this.label, required this.address, this.maxLines});
  final String label;
  final String address;
  final int? maxLines;

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        KEyebrow(label),
        const SizedBox(height: 3),
        Text(
          address.isEmpty ? '—' : address,
          maxLines: maxLines,
          overflow: maxLines == null ? null : TextOverflow.ellipsis,
          style: TextStyle(fontSize: 14, fontWeight: FontWeight.w600, height: 1.3, color: kProviderText),
        ),
      ],
    );
  }
}

class _DashPainter extends CustomPainter {
  @override
  void paint(Canvas canvas, Size size) {
    final paint = Paint()
      ..color = kProviderGreenSoft
      ..strokeWidth = 1.6;
    final x = size.width / 2;
    var y = 4.0;
    while (y < size.height - 4) {
      canvas.drawLine(Offset(x, y), Offset(x, y + 4), paint);
      y += 8;
    }
  }

  @override
  bool shouldRepaint(covariant CustomPainter oldDelegate) => false;
}

// ─── Money & stats ───────────────────────────────────────────────────────────

/// Navy card for balances and fares.
class KMoneyCard extends StatelessWidget {
  const KMoneyCard({super.key, required this.child, this.padding = const EdgeInsets.all(20)});
  final Widget child;
  final EdgeInsetsGeometry padding;

  @override
  Widget build(BuildContext context) {
    return Container(
      width: double.infinity,
      padding: padding,
      decoration: BoxDecoration(
        gradient: kProviderBalanceGradient,
        borderRadius: BorderRadius.circular(22),
        boxShadow: const [BoxShadow(color: Color(0x290B1F44), blurRadius: 24, offset: Offset(0, 10))],
      ),
      child: DefaultTextStyle.merge(style: const TextStyle(color: Colors.white), child: child),
    );
  }
}

/// Small stat card: eyebrow with icon, big value, caption.
class KStatTile extends StatelessWidget {
  const KStatTile({
    super.key,
    required this.label,
    required this.value,
    this.caption,
    this.icon,
    this.onTap,
  });

  final String label;
  final String value;
  final String? caption;
  final IconData? icon;
  final VoidCallback? onTap;

  @override
  Widget build(BuildContext context) {
    return KCard(
      onTap: onTap,
      padding: const EdgeInsets.all(14),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              if (icon != null) ...[Icon(icon, size: 13, color: kTealText), const SizedBox(width: 5)],
              Expanded(child: KEyebrow(label)),
            ],
          ),
          const SizedBox(height: 10),
          FittedBox(
            fit: BoxFit.scaleDown,
            alignment: Alignment.centerLeft,
            child: Text(
              value,
              style: TextStyle(fontSize: 21, fontWeight: FontWeight.w700, letterSpacing: -0.4, color: kProviderText),
            ),
          ),
          if (caption != null) ...[
            const SizedBox(height: 4),
            Text(caption!, maxLines: 1, overflow: TextOverflow.ellipsis, style: TextStyle(fontSize: 12, color: kProviderMuted)),
          ],
        ],
      ),
    );
  }
}

// ─── Progress ────────────────────────────────────────────────────────────────

/// Segmented onboarding progress: filled teal segments up to [current].
class KProgressSteps extends StatelessWidget {
  const KProgressSteps({super.key, required this.total, required this.current});
  final int total;
  final int current;

  @override
  Widget build(BuildContext context) {
    return Row(
      children: [
        for (var i = 0; i < total; i++) ...[
          if (i > 0) const SizedBox(width: 6),
          Expanded(
            child: AnimatedContainer(
              duration: const Duration(milliseconds: 250),
              height: 4,
              decoration: BoxDecoration(
                color: i < current ? kTeal : kProviderBorder,
                borderRadius: BorderRadius.circular(4),
              ),
            ),
          ),
        ],
      ],
    );
  }
}

// ─── People ──────────────────────────────────────────────────────────────────

/// Round avatar: network photo when available, otherwise initials on teal tint.
class KAvatar extends StatelessWidget {
  const KAvatar({super.key, required this.name, this.photoUrl, this.size = 44});
  final String name;
  final String? photoUrl;
  final double size;

  static String initials(String name) {
    final parts = name.trim().split(RegExp(r'\s+')).where((p) => p.isNotEmpty).toList();
    if (parts.isEmpty) return '?';
    if (parts.length == 1) return parts.first.substring(0, 1).toUpperCase();
    return (parts.first.substring(0, 1) + parts.last.substring(0, 1)).toUpperCase();
  }

  @override
  Widget build(BuildContext context) {
    final fallback = Container(
      width: size,
      height: size,
      alignment: Alignment.center,
      decoration: BoxDecoration(color: kTealTint, shape: BoxShape.circle),
      child: Text(
        initials(name),
        style: TextStyle(fontSize: size * 0.34, fontWeight: FontWeight.w700, color: kTealText),
      ),
    );
    final url = photoUrl;
    if (url == null || url.isEmpty) return fallback;
    return ClipOval(
      child: Image.network(
        url,
        width: size,
        height: size,
        fit: BoxFit.cover,
        errorBuilder: (_, _, _) => fallback,
      ),
    );
  }
}

// ─── States ──────────────────────────────────────────────────────────────────

/// The animated illustrations that stand in for icons on empty, error,
/// waiting and success states.
enum KArt { truck, wallet, bell, cloud, success, pending, chat }

/// Picks the animation that fits a state's icon, so every [KEmptyState]
/// gets one without each screen choosing.
KArt kArtFor(IconData icon) {
  final c = icon.codePoint;
  bool any(List<IconData> icons) => icons.any((i) => i.codePoint == c);
  if (any([Icons.cloud_off_rounded, Icons.wifi_off_rounded, Icons.error_outline_rounded])) return KArt.cloud;
  if (any([Icons.notifications_none_rounded, Icons.notifications_rounded])) return KArt.bell;
  if (any([
    Icons.receipt_long_outlined,
    Icons.account_balance_rounded,
    Icons.account_balance_outlined,
    Icons.account_balance_wallet_outlined,
  ])) {
    return KArt.wallet;
  }
  if (any([Icons.chat_bubble_outline_rounded, Icons.forum_outlined, Icons.report_gmailerrorred_rounded])) {
    return KArt.chat;
  }
  if (any([Icons.hourglass_top_rounded, Icons.schedule_rounded])) return KArt.pending;
  if (any([Icons.check_rounded, Icons.check_circle_rounded, Icons.verified_rounded])) return KArt.success;
  return KArt.truck;
}

/// A looping Karryam animation, in the variant drawn for the current theme
/// and the surface it sits on.
class KAnimArt extends StatelessWidget {
  const KAnimArt(this.art, {super.key, this.size = 150, this.onCard = false});
  final KArt art;
  final double size;

  /// Sits on a card rather than the page background (success art only).
  final bool onCard;

  @override
  Widget build(BuildContext context) {
    final theme = KPalette.dark ? 'dark' : 'light';
    final card = onCard && art == KArt.success ? '_card' : '';
    return ExcludeSemantics(
      child: Image.asset(
        'assets/anim/${art.name}_$theme$card.gif',
        width: size,
        height: size,
        gaplessPlayback: true,
        filterQuality: FilterQuality.medium,
      ),
    );
  }
}

class KEmptyState extends StatelessWidget {
  const KEmptyState({
    super.key,
    required this.icon,
    this.art,
    required this.title,
    this.message,
    this.actionLabel,
    this.onAction,
  });

  final IconData icon;

  /// Overrides the animation picked from [icon].
  final KArt? art;
  final String title;
  final String? message;
  final String? actionLabel;
  final VoidCallback? onAction;

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.symmetric(horizontal: 32, vertical: 40),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          KAnimArt(art ?? kArtFor(icon), size: 150),
          const SizedBox(height: 14),
          Text(
            title,
            textAlign: TextAlign.center,
            style: TextStyle(fontSize: 17, fontWeight: FontWeight.w700, color: kProviderText),
          ),
          if (message != null) ...[
            const SizedBox(height: 6),
            Text(
              message!,
              textAlign: TextAlign.center,
              style: TextStyle(fontSize: 13.5, height: 1.45, color: kProviderMuted),
            ),
          ],
          if (actionLabel != null && onAction != null) ...[
            const SizedBox(height: 20),
            SizedBox(width: 200, child: KSecondaryButton(label: actionLabel!, onPressed: onAction)),
          ],
        ],
      ),
    );
  }
}

/// Inline notice: a tinted rounded box with an icon and text.
class KNotice extends StatelessWidget {
  const KNotice(this.message, {super.key, this.tone = KTone.info, this.icon});
  final String message;
  final KTone tone;
  final IconData? icon;

  @override
  Widget build(BuildContext context) {
    final (fg, bg) = switch (tone) {
      KTone.success => (kTealText, kTealTint),
      KTone.info => (kProviderText, kProviderGreenTint),
      KTone.warning => (kWarning, kWarningTint),
      KTone.danger => (kProviderRejectText, kProviderRejectBg),
      KTone.neutral => (kProviderMuted, kProviderGreenTint),
    };
    return Container(
      width: double.infinity,
      padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 12),
      decoration: BoxDecoration(color: bg, borderRadius: BorderRadius.circular(14)),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Icon(
            icon ??
                switch (tone) {
                  KTone.danger => Icons.error_outline_rounded,
                  KTone.warning => Icons.warning_amber_rounded,
                  KTone.success => Icons.check_circle_outline_rounded,
                  _ => Icons.info_outline_rounded,
                },
            size: 18,
            color: fg,
          ),
          const SizedBox(width: 10),
          Expanded(child: Text(message, style: TextStyle(fontSize: 13, height: 1.4, color: fg))),
        ],
      ),
    );
  }
}

/// Label above a form field, in the prototype's style.
class KFieldLabel extends StatelessWidget {
  const KFieldLabel(this.text, {super.key});
  final String text;

  @override
  Widget build(BuildContext context) => Padding(
        padding: const EdgeInsets.only(bottom: 8),
        child: Text(text, style: TextStyle(fontSize: 13, fontWeight: FontWeight.w600, color: kProviderText)),
      );
}

/// Key–value row used in detail cards.
class KInfoRow extends StatelessWidget {
  KInfoRow(this.label, this.value, {super.key, Color? valueColor, this.bold = false})
      : valueColor = valueColor ?? kProviderText;
  final String label;
  final String value;
  final Color valueColor;
  final bool bold;

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 7),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(label, style: TextStyle(fontSize: 13, color: kProviderMuted)),
          const SizedBox(width: 16),
          Expanded(
            child: Text(
              value,
              textAlign: TextAlign.right,
              style: TextStyle(
                fontSize: 13.5,
                fontWeight: bold ? FontWeight.w700 : FontWeight.w600,
                color: valueColor,
              ),
            ),
          ),
        ],
      ),
    );
  }
}

// ─── Illustration ────────────────────────────────────────────────────────────

/// The Karryam box truck, drawn in code so it stays crisp at any size:
/// a pale slatted cargo box, teal cab, navy chassis and wheels.
class KTruckIllustration extends StatelessWidget {
  const KTruckIllustration({super.key, this.width = 220, this.onDark = false});
  final double width;

  /// Lightens the outline colours for use on a navy background.
  final bool onDark;

  @override
  Widget build(BuildContext context) => CustomPaint(
        size: Size(width, width * 0.42),
        painter: _TruckPainter(onDark: onDark),
      );
}

class _TruckPainter extends CustomPainter {
  _TruckPainter({required this.onDark});
  final bool onDark;

  @override
  void paint(Canvas canvas, Size size) {
    final w = size.width;
    final h = size.height;
    final chassis = onDark ? Colors.white : kNavy;
    final box = onDark ? const Color(0x33FFFFFF) : const Color(0xFFDCE9E7);
    final slat = onDark ? const Color(0x55FFFFFF) : const Color(0xFFB8D3CF);

    // Ground shadow.
    canvas.drawOval(
      Rect.fromLTWH(w * 0.04, h * 0.9, w * 0.92, h * 0.08),
      Paint()..color = (onDark ? Colors.black : kNavy).withValues(alpha: 0.08),
    );

    // Cargo box with slats.
    final boxRect = RRect.fromRectAndRadius(Rect.fromLTWH(0, 0, w * 0.66, h * 0.7), Radius.circular(w * 0.02));
    canvas.drawRRect(boxRect, Paint()..color = box);
    final slatPaint = Paint()
      ..color = slat
      ..strokeWidth = w * 0.012;
    for (var i = 1; i < 7; i++) {
      final x = w * 0.66 * i / 7;
      canvas.drawLine(Offset(x, h * 0.12), Offset(x, h * 0.58), slatPaint);
    }

    // Cab.
    final cab = Path()
      ..moveTo(w * 0.68, h * 0.24)
      ..lineTo(w * 0.86, h * 0.24)
      ..lineTo(w * 0.97, h * 0.48)
      ..lineTo(w * 0.97, h * 0.7)
      ..lineTo(w * 0.68, h * 0.7)
      ..close();
    canvas.drawPath(cab, Paint()..color = kTeal);
    final window = Path()
      ..moveTo(w * 0.79, h * 0.31)
      ..lineTo(w * 0.85, h * 0.31)
      ..lineTo(w * 0.91, h * 0.46)
      ..lineTo(w * 0.79, h * 0.46)
      ..close();
    canvas.drawPath(window, Paint()..color = kNavy);

    // Chassis.
    canvas.drawRRect(
      RRect.fromRectAndRadius(Rect.fromLTWH(0, h * 0.7, w, h * 0.05), Radius.circular(h * 0.02)),
      Paint()..color = chassis,
    );

    // Wheels.
    for (final cx in [w * 0.17, w * 0.8]) {
      final c = Offset(cx, h * 0.8);
      canvas.drawCircle(c, h * 0.13, Paint()..color = kNavy);
      canvas.drawCircle(c, h * 0.06, Paint()..color = onDark ? kNavyDeep : Colors.white);
      if (onDark) {
        canvas.drawCircle(
          c,
          h * 0.13,
          Paint()
            ..color = Colors.white
            ..style = PaintingStyle.stroke
            ..strokeWidth = h * 0.025,
        );
      }
    }
  }

  @override
  bool shouldRepaint(covariant _TruckPainter oldDelegate) => oldDelegate.onDark != onDark;
}

/// Navy brand panel with the Karryam logo — the top of the sign-in screens.
class KBrandHero extends StatelessWidget {
  const KBrandHero({super.key, required this.child, this.showTruck = true});
  final Widget child;
  final bool showTruck;

  @override
  Widget build(BuildContext context) {
    final top = MediaQuery.of(context).padding.top;
    return Container(
      width: double.infinity,
      padding: EdgeInsets.fromLTRB(24, top + 20, 24, 28),
      decoration: const BoxDecoration(
        gradient: LinearGradient(
          begin: Alignment.topLeft,
          end: Alignment.bottomRight,
          colors: [Color(0xFF173468), kNavy, kNavyDeep],
        ),
        borderRadius: BorderRadius.vertical(bottom: Radius.circular(28)),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Image.asset('assets/brand/karryam-logo.png', height: 30, semanticLabel: 'Karryam'),
          if (showTruck) ...[
            const SizedBox(height: 26),
            const Center(child: KTruckIllustration(width: 200, onDark: true)),
          ],
          const SizedBox(height: 22),
          DefaultTextStyle.merge(style: const TextStyle(color: Colors.white), child: child),
        ],
      ),
    );
  }
}

// ─── Choice card ─────────────────────────────────────────────────────────────

/// A selectable option: icon tile, title, description and a check circle.
class KChoiceCard extends StatelessWidget {
  const KChoiceCard({
    super.key,
    required this.icon,
    required this.title,
    required this.selected,
    required this.onTap,
    this.subtitle,
  });

  final IconData icon;
  final String title;
  final String? subtitle;
  final bool selected;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return Semantics(
      selected: selected,
      button: true,
      child: AnimatedContainer(
        duration: const Duration(milliseconds: 160),
        decoration: BoxDecoration(
          color: selected ? kSelectedWash : kCard,
          borderRadius: kCardRadius,
          border: Border.all(color: selected ? kTeal : kProviderBorder, width: selected ? 1.8 : 1),
        ),
        child: Material(
          type: MaterialType.transparency,
          child: InkWell(
            onTap: onTap,
            borderRadius: kCardRadius,
            child: Padding(
              padding: const EdgeInsets.all(16),
              child: Row(
                children: [
                  KIconTile(
                    icon,
                    size: 46,
                    color: selected ? kNavy : kTealText,
                    background: selected ? kTeal : kTealTint,
                  ),
                  const SizedBox(width: 14),
                  Expanded(
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Text(title, style: TextStyle(fontSize: 15, fontWeight: FontWeight.w600, color: kProviderText)),
                        if (subtitle != null) ...[
                          const SizedBox(height: 3),
                          Text(subtitle!, style: TextStyle(fontSize: 12.5, height: 1.4, color: kProviderMuted)),
                        ],
                      ],
                    ),
                  ),
                  const SizedBox(width: 10),
                  AnimatedContainer(
                    duration: const Duration(milliseconds: 160),
                    width: 22,
                    height: 22,
                    decoration: BoxDecoration(
                      shape: BoxShape.circle,
                      color: selected ? kTealText : Colors.transparent,
                      border: Border.all(color: selected ? kTealText : kProviderGreenSoft, width: 2),
                    ),
                    child: selected ? Icon(Icons.check_rounded, size: 14, color: kCard) : null,
                  ),
                ],
              ),
            ),
          ),
        ),
      ),
    );
  }
}
