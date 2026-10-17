import 'package:flutter/material.dart';
import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import '../../../../core/theme/theme_extensions.dart';
import '../widgets/driver_pin.dart';

Future<ApiResult<T>> showGoOnlineProcessing<T>(
  BuildContext context, {
  required Future<ApiResult<T>> task,
}) async {
  final safeTask = task.then<ApiResult<T>>(
    (result) => result,
    onError: (Object error) => ApiFailure<T>(ApiException.network(error)),
  );
  await showModalBottomSheet<void>(
    context: context,
    isDismissible: false,
    enableDrag: false,
    useSafeArea: true,
    isScrollControlled: true,
    barrierColor: Colors.black.withValues(alpha: .4),
    builder: (_) => _ConnectingSheet(task: safeTask),
  );
  return safeTask;
}

class _ConnectingSheet extends StatefulWidget {
  const _ConnectingSheet({required this.task});
  final Future<dynamic> task;
  @override
  State<_ConnectingSheet> createState() => _ConnectingSheetState();
}

class _ConnectingSheetState extends State<_ConnectingSheet> {
  @override
  void initState() {
    super.initState();
    _wait();
  }

  Future<void> _wait() async {
    await widget.task;
    if (!mounted) return;
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (mounted) Navigator.of(context).pop();
    });
  }

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    return PopScope(
      canPop: false,
      child: Padding(
        padding: EdgeInsets.fromLTRB(
          24,
          12,
          24,
          28 + MediaQuery.paddingOf(context).bottom,
        ),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Container(
              width: 36,
              height: 4,
              decoration: BoxDecoration(
                color: c.divider,
                borderRadius: BorderRadius.circular(4),
              ),
            ),
            const SizedBox(height: 12),
            const SizedBox.square(
              dimension: 100,
              child: DriverPin(online: true),
            ),
            Text(
              'Connecting you',
              style: Theme.of(context).textTheme.headlineSmall,
            ),
            const SizedBox(height: 10),
            Text(
              'Requesting availability and checking your location.',
              textAlign: TextAlign.center,
              style: Theme.of(context).textTheme.bodyLarge,
            ),
            const SizedBox(height: 24),
            Semantics(
              label: 'Availability request in progress',
              child: ClipRRect(
                borderRadius: BorderRadius.circular(8),
                child: LinearProgressIndicator(
                  minHeight: 3,
                  color: c.brandTeal600,
                  backgroundColor: c.divider,
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }
}
