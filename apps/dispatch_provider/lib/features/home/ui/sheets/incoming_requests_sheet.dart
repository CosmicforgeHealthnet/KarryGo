import 'package:flutter/material.dart';
import '../../../requests/data/request_model.dart';
import '../../../../core/theme/theme_extensions.dart';
import '../../../../shared/widgets/async_content.dart';
import '../widgets/request_card.dart';

class IncomingRequestsSheet extends StatefulWidget {
  const IncomingRequestsSheet({
    super.key,
    required this.requests,
    required this.onAccept,
    required this.onReject,
    this.isLoading = false,
    this.busy = false,
  });
  final List<RequestModel> requests;
  final ValueChanged<RequestModel> onAccept;
  final ValueChanged<RequestModel> onReject;
  final bool isLoading;
  final bool busy;
  @override
  State<IncomingRequestsSheet> createState() => _IncomingRequestsSheetState();
}

class _IncomingRequestsSheetState extends State<IncomingRequestsSheet> {
  String? _selectedId;
  final _pages = ScrollController();
  double _pageWidth = 1;
  @override
  void initState() {
    super.initState();
    _pages.addListener(_trackPage);
  }

  void _trackPage() {
    if (!_pages.hasClients || widget.requests.isEmpty) return;
    final i = (_pages.offset / _pageWidth).round().clamp(
      0,
      widget.requests.length - 1,
    );
    if (_selectedId != widget.requests[i].id)
      setState(() => _selectedId = widget.requests[i].id);
  }

  void _browse(int i) {
    if (!_pages.hasClients) return;
    _pages.animateTo(
      i * _pageWidth,
      duration: Duration(
        milliseconds: MediaQuery.disableAnimationsOf(context) ? 0 : 300,
      ),
      curve: Curves.easeOutCubic,
    );
  }

  @override
  void didUpdateWidget(covariant IncomingRequestsSheet old) {
    super.didUpdateWidget(old);
    if (widget.requests.isEmpty) return;
    final selected = widget.requests.indexWhere((r) => r.id == _selectedId);
    final changed =
        old.requests.length != widget.requests.length ||
        List.generate(
              widget.requests.length,
              (i) => widget.requests[i].id,
            ).join('|') !=
            old.requests.map((r) => r.id).join('|');
    if (changed)
      WidgetsBinding.instance.addPostFrameCallback((_) {
        if (mounted && _pages.hasClients)
          _pages.jumpTo((selected < 0 ? 0 : selected) * _pageWidth);
      });
  }

  @override
  void dispose() {
    _pages.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    final found = widget.requests.indexWhere((r) => r.id == _selectedId);
    final index = found < 0 ? 0 : found;
    return Container(
      margin: const EdgeInsets.fromLTRB(12, 0, 12, 12),
      padding: const EdgeInsets.all(12),
      decoration: BoxDecoration(
        color: c.surface,
        borderRadius: BorderRadius.circular(28),
        border: Border.all(color: c.divider),
        boxShadow: c.cardShadow,
      ),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Padding(
            padding: const EdgeInsets.fromLTRB(6, 2, 0, 12),
            child: Row(
              children: [
                Expanded(
                  child: Text(
                    'Delivery opportunities',
                    style: Theme.of(context).textTheme.titleSmall,
                  ),
                ),
                if (widget.requests.length > 1) ...[
                  Text(
                    '${index + 1}/${widget.requests.length}',
                    style: Theme.of(context).textTheme.bodySmall,
                  ),
                  IconButton(
                    tooltip: 'Previous request',
                    onPressed: widget.busy || index == 0
                        ? null
                        : () => _browse(index - 1),
                    icon: const Icon(Icons.chevron_left_rounded),
                  ),
                  IconButton(
                    tooltip: 'Next request',
                    onPressed:
                        widget.busy || index >= widget.requests.length - 1
                        ? null
                        : () => _browse(index + 1),
                    icon: const Icon(Icons.chevron_right_rounded),
                  ),
                ],
              ],
            ),
          ),
          if (widget.isLoading && widget.requests.isEmpty)
            const SizedBox(height: 140, child: ContentSkeleton(rows: 2))
          else if (widget.requests.isNotEmpty)
            LayoutBuilder(
              builder: (context, constraints) {
                if (_pageWidth != constraints.maxWidth && _pages.hasClients) {
                  WidgetsBinding.instance.addPostFrameCallback((_) {
                    if (mounted && _pages.hasClients)
                      _pages.jumpTo(index * _pageWidth);
                  });
                }
                _pageWidth = constraints.maxWidth;
                return SingleChildScrollView(
                  key: const Key('home-request-pager'),
                  controller: _pages,
                  scrollDirection: Axis.horizontal,
                  physics: widget.busy
                      ? const NeverScrollableScrollPhysics()
                      : const PageScrollPhysics(),
                  child: Row(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      for (final request in widget.requests)
                        SizedBox(
                          width: _pageWidth,
                          child: Padding(
                            padding: const EdgeInsets.symmetric(horizontal: 1),
                            child: RequestCard(
                              key: ValueKey(request.id),
                              request: request,
                              busy: widget.busy,
                              onAccept: () => widget.onAccept(request),
                              onReject: () => widget.onReject(request),
                            ),
                          ),
                        ),
                    ],
                  ),
                );
              },
            )
          else
            const Padding(
              padding: EdgeInsets.all(16),
              child: Text('No requests available right now.'),
            ),
        ],
      ),
    );
  }
}
