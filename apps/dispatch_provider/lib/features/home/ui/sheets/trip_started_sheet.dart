import 'package:flutter/material.dart';
import '../../../requests/data/request_model.dart';
import 'trip_sheet.dart';

class TripStartedSheet extends StatelessWidget {
  const TripStartedSheet({
    super.key,
    required this.request,
    required this.onEndTrip,
    required this.collapsed,
    required this.onToggleCollapse,
    this.peekStatusText = '',
    this.onReportFailure,
    this.busy = false,
  });
  final RequestModel request;
  final VoidCallback onEndTrip, onToggleCollapse;
  final VoidCallback? onReportFailure;
  final bool collapsed, busy;
  final String peekStatusText;
  @override
  Widget build(BuildContext context) => TripSheet(
    request: request,
    title: 'On to the delivery',
    subtitle:
        'Follow the delivery instructions. Collect the required handover evidence before completing.',
    showCall: false,
    primaryLabel: 'Complete delivery',
    primaryEnabled: true,
    secondaryLabel: 'Report delivery issue',
    onPrimary: onEndTrip,
    onSecondary: onReportFailure ?? () {},
    collapsed: collapsed,
    onToggleCollapse: onToggleCollapse,
    peekStatusText: peekStatusText,
    busy: busy,
  );
}
