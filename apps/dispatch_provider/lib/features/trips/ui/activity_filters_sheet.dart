import 'package:flutter/material.dart';


/// Public tab enum shared between the Activity screen and the filter sheet.
enum ActivityTab { all, active, completed, cancelled, failed }

class ActivityFilterResult {
  const ActivityFilterResult({
    required this.tab,
    this.dateRange = 'All time',
    this.location = 'All locations',
  });
  final ActivityTab tab;
  final String dateRange;
  final String location;
}

Future<ActivityFilterResult?> showActivityFiltersSheet(
  BuildContext context, {
  ActivityFilterResult? initial,
}) => showModalBottomSheet<ActivityFilterResult>(
  context: context,
  isScrollControlled: true,
  useSafeArea: true,
  showDragHandle: true,
  barrierColor: Colors.black.withValues(alpha: .5),
  builder: (_) => _ActivityFiltersSheet(initial: initial),
);

class _ActivityFiltersSheet extends StatefulWidget {
  const _ActivityFiltersSheet({this.initial});
  final ActivityFilterResult? initial;
  @override
  State<_ActivityFiltersSheet> createState() => _ActivityFiltersSheetState();
}

class _ActivityFiltersSheetState extends State<_ActivityFiltersSheet> {
  late ActivityTab _status = widget.initial?.tab ?? ActivityTab.all;
  late String _dateRange = widget.initial?.dateRange ?? 'All time';
  @override
  Widget build(BuildContext context) => SingleChildScrollView(
    padding: EdgeInsets.fromLTRB(
      24,
      8,
      24,
      24 + MediaQuery.paddingOf(context).bottom,
    ),
    child: Column(
      mainAxisSize: MainAxisSize.min,
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Text(
          'Filter activity',
          style: Theme.of(context).textTheme.headlineSmall,
        ),
        const SizedBox(height: 20),
        Text('Delivery status', style: Theme.of(context).textTheme.titleSmall),
        const SizedBox(height: 10),
        Wrap(
          spacing: 8,
          runSpacing: 8,
          children: [
            for (final entry in const {
              ActivityTab.all: 'All',
              ActivityTab.active: 'Active',
              ActivityTab.completed: 'Completed',
              ActivityTab.cancelled: 'Cancelled',
              ActivityTab.failed: 'Failed',
            }.entries)
              ChoiceChip(
                label: Text(entry.value),
                selected: _status == entry.key,
                showCheckmark: false,
                onSelected: (_) => setState(() => _status = entry.key),
              ),
          ],
        ),
        const SizedBox(height: 24),
        Text('Date range', style: Theme.of(context).textTheme.titleSmall),
        const SizedBox(height: 10),
        Wrap(
          spacing: 8,
          runSpacing: 8,
          children: [
            for (final label in [
              'Today',
              'Last 7 days',
              'Last 30 days',
              'This year',
              'All time',
            ])
              ChoiceChip(
                label: Text(label),
                selected: _dateRange == label,
                showCheckmark: false,
                onSelected: (_) => setState(() => _dateRange = label),
              ),
          ],
        ),
        const SizedBox(height: 24),
        FilledButton(
          onPressed: () => Navigator.pop(
            context,
            ActivityFilterResult(tab: _status, dateRange: _dateRange),
          ),
          child: const Text('Apply filters'),
        ),
        TextButton(
          onPressed: () => setState(() {
            _status = ActivityTab.all;
            _dateRange = 'All time';
          }),
          child: const Text('Reset filters'),
        ),
      ],
    ),
  );
}
