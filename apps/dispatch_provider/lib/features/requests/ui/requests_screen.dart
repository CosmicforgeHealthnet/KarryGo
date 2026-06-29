import 'package:flutter/material.dart';

import '../state/requests_controller.dart';
import '../../home/ui/home_screen.dart';
import 'request_detail_screen.dart';

class RequestsScreen extends StatefulWidget {
  const RequestsScreen({super.key, required this.requestsController});

  final RequestsController requestsController;

  @override
  State<RequestsScreen> createState() => _RequestsScreenState();
}

class _RequestsScreenState extends State<RequestsScreen> {
  @override
  void initState() {
    super.initState();
    widget.requestsController.addListener(_onControllerChanged);
    _load();
  }

  @override
  void dispose() {
    widget.requestsController.removeListener(_onControllerChanged);
    super.dispose();
  }

  void _onControllerChanged() => setState(() {});

  Future<void> _load() => widget.requestsController.loadRequests();

  @override
  Widget build(BuildContext context) {
    final ctrl = widget.requestsController;
    return Scaffold(
      backgroundColor: const Color(0xFFF7F7F7),
      body: SafeArea(
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            const Padding(
              padding: EdgeInsets.fromLTRB(20, 24, 20, 16),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    'Incoming Requests',
                    style: TextStyle(
                      fontSize: 22,
                      fontWeight: FontWeight.w800,
                      color: Color(0xFF1A1A1A),
                    ),
                  ),
                  SizedBox(height: 4),
                  Text(
                    'Manage all your requests in one place. Accept a\nrequest to start a trip.',
                    style: TextStyle(
                      fontSize: 13,
                      color: Color(0xFF888888),
                      height: 1.5,
                    ),
                  ),
                ],
              ),
            ),
            Expanded(child: _buildBody(ctrl)),
          ],
        ),
      ),
    );
  }

  Widget _buildBody(RequestsController ctrl) {
    if (ctrl.isLoading) {
      return const Center(child: CircularProgressIndicator());
    }
    if (ctrl.error != null) {
      return Center(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Text(
              ctrl.error!,
              textAlign: TextAlign.center,
              style: const TextStyle(fontSize: 14, color: Color(0xFF888888)),
            ),
            const SizedBox(height: 16),
            FilledButton(
              onPressed: () {
                ctrl.clearError();
                _load();
              },
              style: FilledButton.styleFrom(
                backgroundColor: const Color(0xFF4CAF50),
                shape: RoundedRectangleBorder(
                  borderRadius: BorderRadius.circular(999),
                ),
              ),
              child: const Text('Retry'),
            ),
          ],
        ),
      );
    }
    if (ctrl.requests.isEmpty) {
      return const Center(
        child: Text(
          'No active requests',
          style: TextStyle(fontSize: 14, color: Color(0xFF888888)),
        ),
      );
    }
    return ListView.separated(
      padding: const EdgeInsets.fromLTRB(16, 0, 16, 100),
      itemCount: ctrl.requests.length,
      separatorBuilder: (_, _) => const SizedBox(height: 12),
      itemBuilder: (context, i) {
        final req = ctrl.requests[i];
        return GestureDetector(
          onTap: () => Navigator.of(context).push(
            MaterialPageRoute(
              builder: (_) => RequestDetailScreen(
                request: req,
                onAccept: () => _onAccept(req),
                onReject: () => _onReject(req),
              ),
            ),
          ),
          child: RequestCard(
            request: req,
            onAccept: () => _onAccept(req),
            onReject: () => _onReject(req),
          ),
        );
      },
    );
  }

  Future<void> _onAccept(RequestModel req) async {
    final result = await widget.requestsController.accept(req.id);
    if (!mounted) return;
    result.when(
      success: (_) {
        ScaffoldMessenger.of(context).showSnackBar(
          const SnackBar(
            content: Text('Request accepted — trip is starting.'),
            backgroundColor: Color(0xFF4CAF50),
          ),
        );
      },
      failure: (err) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(
            content: Text(
              err.message.isNotEmpty
                  ? err.message
                  : 'Failed to accept request.',
            ),
            backgroundColor: Colors.red.shade800,
          ),
        );
      },
    );
  }

  Future<void> _onReject(RequestModel req) async {
    final result = await widget.requestsController.reject(
      req.id,
      'Provider rejected',
    );
    if (!mounted) return;
    result.when(
      success: (_) {},
      failure: (err) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(
            content: Text(
              err.message.isNotEmpty
                  ? err.message
                  : 'Failed to reject request.',
            ),
            backgroundColor: Colors.red.shade800,
          ),
        );
      },
    );
  }
}
