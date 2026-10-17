import 'package:flutter/material.dart';
import '../../core/theme/theme_extensions.dart';
import 'delivery_collection_widgets.dart';
import 'dispatch_screen_frame.dart';
import 'journey_icon.dart';

class AccountPage extends StatelessWidget {
  const AccountPage({
    super.key,
    required this.title,
    required this.symbol,
    required this.children,
    this.description,
    this.bottom,
    this.actions,
    this.onRefresh,
    this.onBack,
  });
  final String title;
  final String? description;
  final JourneySymbol symbol;
  final List<Widget> children;
  final Widget? bottom;
  final List<Widget>? actions;
  final Future<void> Function()? onRefresh;
  final VoidCallback? onBack;
  @override
  Widget build(BuildContext context) {
    Widget content = ListView(
      physics: const AlwaysScrollableScrollPhysics(),
      padding: const EdgeInsets.fromLTRB(20, 12, 20, 28),
      children: [
        Center(
          child: ConstrainedBox(
            constraints: const BoxConstraints(maxWidth: 600),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                Align(
                  alignment: Alignment.centerLeft,
                  child: AccountEmblem(symbol: symbol, size: 58),
                ),
                const SizedBox(height: 20),
                Text(title, style: Theme.of(context).textTheme.headlineMedium),
                if (description != null) ...[
                  const SizedBox(height: 8),
                  Text(
                    description!,
                    style: Theme.of(context).textTheme.bodyMedium,
                  ),
                ],
                const SizedBox(height: 26),
                ...children,
              ],
            ),
          ),
        ),
      ],
    );
    if (onRefresh != null)
      content = RefreshIndicator(onRefresh: onRefresh!, child: content);
    return Scaffold(
      backgroundColor: context.palette.background,
      appBar: AppBar(
        leading: DispatchBackButton(onPressed: onBack),
        actions: actions,
      ),
      body: content,
      bottomNavigationBar: bottom == null
          ? null
          : SafeArea(
              top: false,
              child: Padding(
                padding: EdgeInsets.fromLTRB(
                  20,
                  12,
                  20,
                  16 + MediaQuery.viewInsetsOf(context).bottom,
                ),
                child: bottom,
              ),
            ),
    );
  }
}

class AccountEmblem extends StatelessWidget {
  const AccountEmblem({
    super.key,
    required this.symbol,
    this.color,
    this.size = 44,
  });
  final JourneySymbol symbol;
  final Color? color;
  final double size;
  @override
  Widget build(BuildContext context) => Container(
    width: size,
    height: size,
    decoration: BoxDecoration(
      color: (color ?? context.palette.brandTeal600).withValues(alpha: .09),
      borderRadius: BorderRadius.circular(size * .32),
    ),
    alignment: Alignment.center,
    child: JourneyIcon(symbol, size: size * .57, color: color),
  );
}

class AccountMenuTile extends StatelessWidget {
  const AccountMenuTile({
    super.key,
    required this.title,
    required this.symbol,
    this.subtitle,
    this.onTap,
    this.color,
    this.trailing,
  });
  final String title;
  final String? subtitle;
  final JourneySymbol symbol;
  final Color? color;
  final VoidCallback? onTap;
  final Widget? trailing;
  @override
  Widget build(BuildContext context) => ListTile(
    contentPadding: const EdgeInsets.symmetric(horizontal: 16, vertical: 8),
    leading: AccountEmblem(symbol: symbol, color: color),
    title: Text(
      title,
      style: Theme.of(context).textTheme.titleSmall?.copyWith(color: color),
    ),
    subtitle: subtitle == null
        ? null
        : Padding(
            padding: const EdgeInsets.only(top: 5),
            child: Text(
              subtitle!,
              style: Theme.of(context).textTheme.bodySmall,
            ),
          ),
    trailing:
        trailing ??
        (onTap == null
            ? null
            : Icon(
                Icons.chevron_right_rounded,
                color: context.palette.textMuted,
              )),
    onTap: onTap,
  );
}

class AccountSection extends StatelessWidget {
  const AccountSection({
    super.key,
    required this.title,
    required this.children,
  });
  final String title;
  final List<Widget> children;
  @override
  Widget build(BuildContext context) => Padding(
    padding: const EdgeInsets.only(bottom: 24),
    child: Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Padding(
          padding: const EdgeInsets.only(left: 4, bottom: 12),
          child: Text(title, style: Theme.of(context).textTheme.titleSmall),
        ),
        DeliveryPanel(
          padding: EdgeInsets.zero,
          child: Column(
            children: [
              for (var i = 0; i < children.length; i++) ...[
                if (i > 0) const Divider(height: 1, indent: 76, endIndent: 16),
                children[i],
              ],
            ],
          ),
        ),
      ],
    ),
  );
}

class AccountValue extends StatelessWidget {
  const AccountValue({super.key, required this.label, required this.value});
  final String label, value;
  @override
  Widget build(BuildContext context) => Padding(
    padding: const EdgeInsets.symmetric(vertical: 12),
    child: Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(label, style: Theme.of(context).textTheme.bodySmall),
        const SizedBox(height: 5),
        SelectableText(
          value.trim().isEmpty ? 'Not provided' : value,
          style: Theme.of(context).textTheme.titleSmall,
        ),
      ],
    ),
  );
}

Future<bool> confirmAccountAction(
  BuildContext context, {
  required String title,
  required String message,
  required String action,
  JourneySymbol symbol = JourneySymbol.shield,
  bool destructive = false,
}) async =>
    await showModalBottomSheet<bool>(
      context: context,
      useSafeArea: true,
      showDragHandle: true,
      isScrollControlled: true,
      barrierColor: Colors.black.withValues(alpha: .5),
      builder: (sheet) => SingleChildScrollView(
        padding: const EdgeInsets.fromLTRB(24, 4, 24, 24),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          mainAxisSize: MainAxisSize.min,
          children: [
            Align(
              alignment: Alignment.centerLeft,
              child: AccountEmblem(
                symbol: symbol,
                size: 60,
                color: destructive ? sheet.palette.danger : null,
              ),
            ),
            const SizedBox(height: 20),
            Text(title, style: Theme.of(sheet).textTheme.headlineSmall),
            const SizedBox(height: 10),
            Text(message),
            const SizedBox(height: 24),
            FilledButton(
              onPressed: () => Navigator.pop(sheet, true),
              style: destructive
                  ? FilledButton.styleFrom(
                      backgroundColor: sheet.palette.danger,
                      foregroundColor: Colors.white,
                    )
                  : null,
              child: Text(action),
            ),
            const SizedBox(height: 8),
            TextButton(
              onPressed: () => Navigator.pop(sheet, false),
              child: const Text('Cancel'),
            ),
          ],
        ),
      ),
    ) ??
    false;
