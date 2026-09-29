import 'package:flutter/material.dart';

import '../../../requests/data/request_model.dart';
import '../../../../core/theme/theme_extensions.dart';
import '../widgets/request_card.dart';

class IncomingRequestsSheet extends StatelessWidget {
  const IncomingRequestsSheet({
    super.key,
    required this.requests,
    required this.onAccept,
    required this.onReject,
    this.isLoading = false,
  });
  final List<RequestModel> requests;
  final ValueChanged<RequestModel> onAccept;
  final ValueChanged<RequestModel> onReject;
  final bool isLoading;

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;
    final bottomPad = MediaQuery.of(context).padding.bottom;

    return Column(
      mainAxisSize: MainAxisSize.min,
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Padding(
          padding: const EdgeInsets.fromLTRB(20, 12, 20, 8),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(
                'Incoming Requests...',
                style: TextStyle(
                  fontSize: 18,
                  fontWeight: FontWeight.w800,
                  color: colors.text,
                ),
              ),
              const SizedBox(height: 4),
              Text(
                'Accept a request to start a trip.',
                style: TextStyle(fontSize: 13, color: colors.textMuted),
              ),
            ],
          ),
        ),
        if (isLoading)
          const Padding(
            padding: EdgeInsets.symmetric(vertical: 32),
            child: Center(child: CircularProgressIndicator()),
          )
        else if (requests.isEmpty)
          Padding(
            padding: const EdgeInsets.symmetric(vertical: 32, horizontal: 20),
            child: Center(
              child: Text(
                'No active requests',
                style: TextStyle(fontSize: 14, color: colors.textMuted),
              ),
            ),
          )
        else
          SizedBox(
            height: 320,
            child: PageView.builder(
              padEnds: false,
              controller: PageController(viewportFraction: 0.88),
              itemCount: requests.length,
              itemBuilder: (context, i) => Padding(
                padding: EdgeInsets.only(
                  left: i == 0 ? 20 : 8,
                  right: i == requests.length - 1 ? 20 : 8,
                ),
                child: RequestCard(
                  request: requests[i],
                  onAccept: () => onAccept(requests[i]),
                  onReject: () => onReject(requests[i]),
                ),
              ),
            ),
          ),
        SizedBox(height: 12 + bottomPad),
      ],
    );
  }
}