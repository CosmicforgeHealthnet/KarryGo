import 'package:flutter/material.dart';
import '../../../core/theme/vendor_theme.dart';
import '../../dashboard/ui/dashboard_format.dart';
import '../state/settings_controller.dart';

Color settingsColor(Object? status) => switch (status) {
  'ACTIVE' ||
  'active' ||
  'VERIFIED' ||
  'READY' ||
  'COMPLETED' => KarryamColors.tealInk,
  'SUSPENDED' ||
  'suspended' ||
  'REJECTED' ||
  'EXPIRED' ||
  'BLOCKED' ||
  'BLACKOUT' ||
  'CLOSED' => KarryamColors.error,
  'HELD' ||
  'ACKNOWLEDGED' ||
  'PAUSED' ||
  'PENDING_REVIEW' ||
  'PENDING_VERIFICATION' => const Color(0xFF986013),
  _ => const Color(0xFF64768A),
};
IconData settingsIcon(SettingsKind kind) => switch (kind) {
  SettingsKind.profile => Icons.business_outlined,
  SettingsKind.brands => Icons.auto_awesome_mosaic_outlined,
  SettingsKind.branches => Icons.storefront_outlined,
  SettingsKind.capabilities => Icons.widgets_outlined,
  SettingsKind.staff => Icons.groups_2_outlined,
  SettingsKind.roles => Icons.admin_panel_settings_outlined,
};
Color settingsAccent(SettingsKind kind) => switch (kind) {
  SettingsKind.brands => const Color(0xFFA66028),
  SettingsKind.staff => const Color(0xFF326AA2),
  SettingsKind.roles => const Color(0xFF7660A6),
  _ => KarryamColors.tealInk,
};

class SettingsAction extends StatelessWidget {
  const SettingsAction(
    this.label, {
    super.key,
    this.onPressed,
    this.busy = false,
    this.secondary = false,
    this.danger = false,
    this.icon = Icons.arrow_forward_rounded,
  });
  final String label;
  final VoidCallback? onPressed;
  final bool busy, secondary, danger;
  final IconData icon;
  @override
  Widget build(BuildContext context) => SizedBox(
    width: double.infinity,
    child: FilledButton(
      style: FilledButton.styleFrom(
        minimumSize: const Size(0, 58),
        padding: const EdgeInsets.symmetric(horizontal: 20, vertical: 16),
        backgroundColor:
            (danger
                    ? KarryamColors.error.withValues(alpha: .08)
                    : secondary
                    ? KarryamColors.mint
                    : KarryamColors.teal)
                .forSurface(context),
        foregroundColor: danger
            ? KarryamColors.error.forInk(context)
            : secondary
            ? KarryamColors.tealInk.forInk(context)
            : KarryamColors.navy,
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(20)),
      ),
      onPressed: busy ? null : onPressed,
      child: Row(
        mainAxisAlignment: MainAxisAlignment.center,
        children: [
          if (busy)
            const SizedBox(
              width: 20,
              height: 20,
              child: CircularProgressIndicator(strokeWidth: 2),
            )
          else
            Icon(icon, size: 20),
          const SizedBox(width: 10),
          Flexible(child: Text(label, textAlign: TextAlign.center)),
        ],
      ),
    ),
  );
}

class SettingsCard extends StatelessWidget {
  const SettingsCard({
    super.key,
    required this.child,
    this.color = Colors.white,
  });
  final Widget child;
  final Color color;
  @override
  Widget build(BuildContext context) => SizedBox(
    width: double.infinity,
    child: Material(
      color: (color).forSurface(context),
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(26),
        side: BorderSide(
          color: (KarryamColors.navy.withValues(
            alpha: .04,
          )).forOutline(context),
        ),
      ),
      child: Padding(padding: const EdgeInsets.all(22), child: child),
    ),
  );
}

class SettingsStatus extends StatelessWidget {
  const SettingsStatus(this.value, {super.key});
  final Object? value;
  @override
  Widget build(BuildContext context) => Container(
    padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 7),
    decoration: BoxDecoration(
      color: (settingsColor(value).withValues(alpha: .08)).forSurface(context),
      borderRadius: BorderRadius.circular(12),
    ),
    child: Text(
      dashboardLabel(value),
      style: TextStyle(
        fontSize: 12,
        fontWeight: FontWeight.w700,
        color: (settingsColor(value)).forInk(context),
      ),
    ),
  );
}

class SettingsNote extends StatelessWidget {
  const SettingsNote(this.message, {super.key, this.warning = false});
  final String message;
  final bool warning;
  @override
  Widget build(BuildContext context) {
    final color = warning ? const Color(0xFF986013) : KarryamColors.error;
    return Container(
      margin: const EdgeInsets.only(bottom: 16),
      padding: const EdgeInsets.all(18),
      decoration: BoxDecoration(
        color: (color.withValues(alpha: .07)).forSurface(context),
        borderRadius: BorderRadius.circular(20),
      ),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Icon(
            warning ? Icons.info_outline_rounded : Icons.error_outline_rounded,
            color: (color).forInk(context),
            size: 21,
          ),
          const SizedBox(width: 12),
          Expanded(
            child: Text(
              message,
              style: TextStyle(color: (color).forInk(context)),
            ),
          ),
        ],
      ),
    );
  }
}

class SettingsHeading extends StatelessWidget {
  const SettingsHeading(this.title, {super.key, this.subtitle});
  final String title;
  final String? subtitle;
  @override
  Widget build(BuildContext context) => Padding(
    padding: const EdgeInsets.only(bottom: 24),
    child: Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(
          title,
          style: const TextStyle(
            fontSize: 30,
            fontWeight: FontWeight.w700,
            letterSpacing: -1,
          ),
        ),
        if (subtitle?.isNotEmpty == true) ...[
          const SizedBox(height: 8),
          Text(subtitle!),
        ],
      ],
    ),
  );
}

class SettingsFact extends StatelessWidget {
  const SettingsFact(this.label, this.value, {super.key});
  final String label;
  final Object? value;
  @override
  Widget build(BuildContext context) => Padding(
    padding: const EdgeInsets.symmetric(vertical: 10),
    child: SizedBox(
      width: double.infinity,
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            label,
            style: TextStyle(
              fontSize: 12,
              color: Color(0xFF647780).forInk(context),
            ),
          ),
          const SizedBox(height: 5),
          SelectableText(
            value?.toString().isNotEmpty == true
                ? value.toString()
                : 'Not provided',
            style: const TextStyle(fontWeight: FontWeight.w600),
          ),
        ],
      ),
    ),
  );
}

class SettingsLink extends StatelessWidget {
  const SettingsLink(
    this.title,
    this.icon, {
    super.key,
    required this.onTap,
    this.subtitle,
    this.color = KarryamColors.tealInk,
  });
  final String title;
  final String? subtitle;
  final IconData icon;
  final Color color;
  final VoidCallback? onTap;
  @override
  Widget build(BuildContext context) => Padding(
    padding: const EdgeInsets.only(bottom: 12),
    child: Material(
      color: Colors.white.forSurface(context),
      borderRadius: BorderRadius.circular(22),
      clipBehavior: Clip.antiAlias,
      child: InkWell(
        onTap: onTap,
        child: Padding(
          padding: const EdgeInsets.all(18),
          child: Row(
            children: [
              Container(
                padding: const EdgeInsets.all(12),
                decoration: BoxDecoration(
                  color: (color.withValues(alpha: .08)).forSurface(context),
                  borderRadius: BorderRadius.circular(16),
                ),
                child: Icon(icon, color: (color).forInk(context), size: 23),
              ),
              const SizedBox(width: 15),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      title,
                      style: const TextStyle(
                        fontWeight: FontWeight.w700,
                        fontSize: 16,
                      ),
                    ),
                    if (subtitle?.isNotEmpty == true) ...[
                      const SizedBox(height: 4),
                      Text(subtitle!, style: const TextStyle(fontSize: 12)),
                    ],
                  ],
                ),
              ),
              Icon(
                Icons.chevron_right_rounded,
                color: Color(0xFF839497).forInk(context),
                size: 21,
              ),
            ],
          ),
        ),
      ),
    ),
  );
}

class SettingsEmblem extends StatelessWidget {
  const SettingsEmblem(this.kind, {super.key, this.size = 88});
  final SettingsKind kind;
  final double size;
  @override
  Widget build(BuildContext context) => ExcludeSemantics(
    child: SizedBox(
      width: size,
      height: size,
      child: CustomPaint(
        painter: _SettingsPainter(
          kind,
          settingsAccent(kind).forInk(context),
          Theme.of(context).colorScheme.surfaceContainerHighest,
        ),
        child: Center(
          child: Icon(
            settingsIcon(kind),
            size: size * .38,
            color: (settingsAccent(kind)).forInk(context),
          ),
        ),
      ),
    ),
  );
}

class _SettingsPainter extends CustomPainter {
  _SettingsPainter(this.kind, this.color, this.surface);
  final Color color, surface;
  final SettingsKind kind;
  @override
  void paint(Canvas canvas, Size size) {
    final center = Offset(size.width / 2, size.height / 2);
    final paint = Paint()..color = color.withValues(alpha: .08);
    canvas.drawCircle(center, size.width * .47, paint);
    canvas.drawRRect(
      RRect.fromRectAndRadius(
        Rect.fromCenter(
          center: center,
          width: size.width * .65,
          height: size.height * .65,
        ),
        const Radius.circular(19),
      ),
      paint..color = surface,
    );
    final edge = Offset(size.width * .83, size.height * .2);
    canvas.drawCircle(
      edge,
      size.width * .13,
      paint..color = color.withValues(alpha: .14),
    );
    canvas.drawLine(
      edge - const Offset(3, 0),
      edge + const Offset(3, 0),
      paint
        ..color = color
        ..strokeWidth = 2
        ..strokeCap = StrokeCap.round,
    );
    canvas.drawLine(
      edge - const Offset(0, 3),
      edge + const Offset(0, 3),
      paint,
    );
    canvas.drawCircle(
      Offset(size.width * .16, size.height * .77),
      size.width * .06,
      paint..color = color.withValues(alpha: .24),
    );
  }

  @override
  bool shouldRepaint(_SettingsPainter oldDelegate) =>
      kind != oldDelegate.kind ||
      color != oldDelegate.color ||
      surface != oldDelegate.surface;
}

Future<T?> settingsSheet<T>(
  BuildContext context,
  Widget child, {
  Widget? footer,
}) => showModalBottomSheet<T>(
  context: context,
  useSafeArea: true,
  showDragHandle: true,
  isScrollControlled: true,
  backgroundColor: KarryamColors.canvas.forSurface(context),
  builder: (context) => Padding(
    padding: EdgeInsets.only(bottom: MediaQuery.viewInsetsOf(context).bottom),
    child: ConstrainedBox(
      constraints: BoxConstraints(
        maxHeight:
            (MediaQuery.sizeOf(context).height * .85 -
                    MediaQuery.viewInsetsOf(context).bottom)
                .clamp(120, double.infinity),
      ),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          Flexible(
            child: SingleChildScrollView(
              padding: const EdgeInsets.fromLTRB(24, 0, 24, 20),
              child: child,
            ),
          ),
          if (footer != null)
            Padding(padding: EdgeInsets.fromLTRB(24, 8, 24, 24), child: footer),
        ],
      ),
    ),
  ),
);
Future<bool> confirmSettings(
  BuildContext context,
  String title,
  String message, {
  bool danger = false,
}) async =>
    await settingsSheet<bool>(
      context,
      Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [SettingsHeading(title), Text(message)],
      ),
      footer: SettingsAction(
        title,
        danger: danger,
        onPressed: () => Navigator.pop(context, true),
      ),
    ) ==
    true;
