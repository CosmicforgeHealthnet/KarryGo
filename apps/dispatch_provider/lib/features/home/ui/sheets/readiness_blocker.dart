import 'package:flutter/material.dart';
import '../../../../core/theme/theme_extensions.dart';
import '../../../../shared/widgets/journey_icon.dart';

enum ReadinessItemStatus { missing, pending }

class ReadinessItem {
  const ReadinessItem({
    required this.title,
    required this.subtitle,
    required this.status,
  });
  final String title;
  final String subtitle;
  final ReadinessItemStatus status;
}

Future<void> showReadinessBlocker(
  BuildContext context, {
  required List<ReadinessItem> items,
  required VoidCallback onCompleteSetup,
  VoidCallback? onMaybeLater,
}) => showModalBottomSheet<void>(
  context: context,
  useSafeArea: true,
  showDragHandle: true,
  isScrollControlled: true,
  builder: (sheet) => SingleChildScrollView(
    child: Padding(
      padding: EdgeInsets.fromLTRB(
        24,
        8,
        24,
        24 + MediaQuery.paddingOf(sheet).bottom,
      ),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Align(
            alignment: Alignment.centerLeft,
            child: Container(
              padding: const EdgeInsets.all(14),
              decoration: BoxDecoration(
                color: sheet.palette.warningSoft,
                borderRadius: BorderRadius.circular(20),
              ),
              child: JourneyIcon(
                JourneySymbol.shield,
                size: 32,
                color: sheet.palette.warning,
              ),
            ),
          ),
          const SizedBox(height: 20),
          Text(
            'A few things to finish',
            style: Theme.of(sheet).textTheme.headlineSmall,
          ),
          const SizedBox(height: 8),
          Text(
            'Review these requirements before going online.',
            style: Theme.of(sheet).textTheme.bodyLarge,
          ),
          const SizedBox(height: 24),
          for (final item in items)
            Padding(
              padding: const EdgeInsets.only(bottom: 20),
              child: _ReadinessRow(item: item),
            ),
          FilledButton(
            onPressed: () {
              Navigator.pop(sheet);
              onCompleteSetup();
            },
            child: const Text('Review requirements'),
          ),
          TextButton(
            onPressed: () {
              Navigator.pop(sheet);
              onMaybeLater?.call();
            },
            child: const Text('Not now'),
          ),
        ],
      ),
    ),
  ),
);

class _ReadinessRow extends StatelessWidget {
  const _ReadinessRow({required this.item});
  final ReadinessItem item;

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;
    final isPending = item.status == ReadinessItemStatus.pending;
    final accent = isPending ? colors.warning : colors.danger;

    return Row(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Container(
          width: 26,
          height: 26,
          margin: const EdgeInsets.only(top: 1),
          decoration: BoxDecoration(color: accent, shape: BoxShape.circle),
          child: Icon(
            isPending ? Icons.access_time_rounded : Icons.close_rounded,
            size: 16,
            color: Colors.white,
          ),
        ),
        const SizedBox(width: 14),
        Expanded(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(
                item.title,
                style: TextStyle(
                  fontSize: 15,
                  fontWeight: FontWeight.w700,
                  color: colors.text,
                  height: 1.2,
                  letterSpacing: -0.1,
                ),
              ),
              const SizedBox(height: 3),
              Text(
                item.subtitle,
                style: TextStyle(
                  fontSize: 13,
                  fontWeight: FontWeight.w400,
                  color: colors.textMuted,
                  height: 1.35,
                ),
              ),
            ],
          ),
        ),
      ],
    );
  }
}
