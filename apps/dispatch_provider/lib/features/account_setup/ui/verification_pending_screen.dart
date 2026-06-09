import 'package:flutter/material.dart';
import '../../verification/state/verification_controller.dart';
import '../../verification/models/verification_models.dart';

class VerificationPendingScreen extends StatefulWidget {
  const VerificationPendingScreen({
    super.key,
    required this.onGoToDashboard,
    required this.verificationController,
  });

  final VoidCallback onGoToDashboard;
  final VerificationController verificationController;

  @override
  State<VerificationPendingScreen> createState() => _VerificationPendingScreenState();
}

class _VerificationPendingScreenState extends State<VerificationPendingScreen> {
  bool _localLoading = false;
  String? _localError;

  @override
  void initState() {
    super.initState();
    _fetchStatus();
  }

  Future<void> _fetchStatus() async {
    if (_localLoading) return;
    setState(() {
      _localLoading = true;
      _localError = null;
    });

    final result = await widget.verificationController.loadVerificationStatus();
    if (mounted) {
      setState(() {
        _localLoading = false;
      });
      result.when(
        success: (_) {},
        failure: (error) {
          setState(() {
            _localError = error.message;
          });
        },
      );
    }
  }

  @override
  Widget build(BuildContext context) {
    final state = widget.verificationController;
    final statusResponse = state.latestStatus;

    return Scaffold(
      backgroundColor: const Color(0xFFF5F5F5),
      appBar: AppBar(
        title: const Text(
          'Verification Status',
          style: TextStyle(
            fontWeight: FontWeight.w800,
            fontSize: 18,
            color: Color(0xFF1A1A1A),
          ),
        ),
        backgroundColor: Colors.white,
        elevation: 0,
        actions: [
          IconButton(
            icon: const Icon(Icons.refresh_rounded, color: Color(0xFF4CAF50)),
            onPressed: _fetchStatus,
            tooltip: 'Refresh Status',
          ),
        ],
      ),
      body: SafeArea(
        child: Padding(
          padding: const EdgeInsets.fromLTRB(24, 20, 24, 20),
          child: _localLoading && statusResponse == null
              ? const Center(
                  child: Column(
                    mainAxisAlignment: MainAxisAlignment.center,
                    children: [
                      CircularProgressIndicator(color: Color(0xFF4CAF50)),
                      SizedBox(height: 16),
                      Text(
                        'Checking verification status...',
                        style: TextStyle(color: Color(0xFF888888)),
                      ),
                    ],
                  ),
                )
              : _localError != null && statusResponse == null
                  ? Center(
                      child: Column(
                        mainAxisAlignment: MainAxisAlignment.center,
                        children: [
                          const Icon(Icons.error_outline_rounded, color: Colors.red, size: 48),
                          const SizedBox(height: 16),
                          Text(
                            'Failed to load status: $_localError',
                            textAlign: TextAlign.center,
                            style: const TextStyle(color: Color(0xFF888888)),
                          ),
                          const SizedBox(height: 16),
                          OutlinedButton.icon(
                            onPressed: _fetchStatus,
                            icon: const Icon(Icons.refresh_rounded),
                            label: const Text('Retry'),
                          ),
                        ],
                      ),
                    )
                  : statusResponse == null
                      ? Center(
                          child: Column(
                            mainAxisAlignment: MainAxisAlignment.center,
                            children: [
                              const Text('No status data available.'),
                              const SizedBox(height: 16),
                              ElevatedButton(
                                onPressed: _fetchStatus,
                                child: const Text('Load Status'),
                              ),
                            ],
                          ),
                        )
                      : Column(
                          crossAxisAlignment: CrossAxisAlignment.stretch,
                          children: [
                            Expanded(
                              child: SingleChildScrollView(
                                child: Column(
                                  crossAxisAlignment: CrossAxisAlignment.stretch,
                                  children: [
                                    Center(
                                      child: Image.asset(
                                        'assets/figma/profile_submitted.png',
                                        width: 140,
                                        height: 130,
                                        fit: BoxFit.contain,
                                        errorBuilder: (context, error, stackTrace) =>
                                            const Icon(Icons.assignment_turned_in_rounded,
                                                size: 80, color: Color(0xFF4CAF50)),
                                      ),
                                    ),
                                    const SizedBox(height: 20),
                                    Text(
                                      _getOverallStatusTitle(statusResponse.overallStatus),
                                      textAlign: TextAlign.center,
                                      style: const TextStyle(
                                        fontSize: 20,
                                        fontWeight: FontWeight.w900,
                                        color: Color(0xFF1A1A1A),
                                      ),
                                    ),
                                    const SizedBox(height: 8),
                                    Text(
                                      _getOverallStatusDescription(statusResponse.overallStatus),
                                      textAlign: TextAlign.center,
                                      style: const TextStyle(
                                        fontSize: 13,
                                        color: Color(0xFF888888),
                                        height: 1.5,
                                      ),
                                    ),
                                    const SizedBox(height: 24),
                                    
                                    // Progress Bar Card
                                    Container(
                                      padding: const EdgeInsets.all(16),
                                      decoration: BoxDecoration(
                                        color: Colors.white,
                                        borderRadius: BorderRadius.circular(16),
                                        boxShadow: [
                                          BoxShadow(
                                            color: Colors.black.withValues(alpha: 0.03),
                                            blurRadius: 10,
                                            offset: const Offset(0, 4),
                                          ),
                                        ],
                                      ),
                                      child: Column(
                                        crossAxisAlignment: CrossAxisAlignment.start,
                                        children: [
                                          Row(
                                            mainAxisAlignment: MainAxisAlignment.spaceBetween,
                                            children: [
                                              const Text(
                                                'Verification Progress',
                                                style: TextStyle(
                                                  fontWeight: FontWeight.w700,
                                                  fontSize: 14,
                                                  color: Color(0xFF1A1A1A),
                                                ),
                                              ),
                                              Text(
                                                '${statusResponse.completionPercentage}%',
                                                style: const TextStyle(
                                                  fontWeight: FontWeight.w800,
                                                  fontSize: 15,
                                                  color: Color(0xFF4CAF50),
                                                ),
                                              ),
                                            ],
                                          ),
                                          const SizedBox(height: 10),
                                          LinearProgressIndicator(
                                            value: statusResponse.completionPercentage / 100.0,
                                            backgroundColor: const Color(0xFFE8F5E9),
                                            color: const Color(0xFF4CAF50),
                                            minHeight: 8,
                                            borderRadius: BorderRadius.circular(4),
                                          ),
                                        ],
                                      ),
                                    ),
                                    const SizedBox(height: 20),

                                    // Steps Card
                                    const Text(
                                      'VERIFICATION STEPS',
                                      style: TextStyle(
                                        fontSize: 11,
                                        fontWeight: FontWeight.w700,
                                        color: Color(0xFF888888),
                                        letterSpacing: 1.0,
                                      ),
                                    ),
                                    const SizedBox(height: 8),
                                    Container(
                                      decoration: BoxDecoration(
                                        color: Colors.white,
                                        borderRadius: BorderRadius.circular(16),
                                        boxShadow: [
                                          BoxShadow(
                                            color: Colors.black.withValues(alpha: 0.03),
                                            blurRadius: 10,
                                            offset: const Offset(0, 4),
                                          ),
                                        ],
                                      ),
                                      child: ListView.separated(
                                        shrinkWrap: true,
                                        physics: const NeverScrollableScrollPhysics(),
                                        itemCount: statusResponse.steps.length,
                                        separatorBuilder: (context, index) =>
                                            const Divider(height: 1, color: Color(0xFFF0F0F0)),
                                        itemBuilder: (context, i) {
                                          final step = statusResponse.steps[i];
                                          return _StepListTile(step: step);
                                        },
                                      ),
                                    ),
                                  ],
                                ),
                              ),
                            ),
                            
                            const SizedBox(height: 12),
                            if (_localLoading)
                              const Center(child: CircularProgressIndicator(color: Color(0xFF4CAF50)))
                            else
                              SizedBox(
                                height: 52,
                                child: FilledButton(
                                  onPressed: statusResponse.overallStatus == 'verified'
                                      ? widget.onGoToDashboard
                                      : null,
                                  style: FilledButton.styleFrom(
                                    backgroundColor: const Color(0xFF4CAF50),
                                    disabledBackgroundColor: const Color(0xFFBCDFCD),
                                    shape: RoundedRectangleBorder(
                                      borderRadius: BorderRadius.circular(999),
                                    ),
                                  ),
                                  child: const Text(
                                    'Go to dashboard',
                                    style: TextStyle(
                                      fontSize: 16,
                                      fontWeight: FontWeight.w700,
                                      color: Colors.white,
                                    ),
                                  ),
                                ),
                              ),
                          ],
                        ),
        ),
      ),
    );
  }

  String _getOverallStatusTitle(String status) {
    switch (status) {
      case 'verified':
        return 'Account Verified!';
      case 'pending_review':
        return 'Review in Progress';
      case 'rejected':
        return 'Verification Rejected';
      case 'suspended':
        return 'Account Suspended';
      case 'in_progress':
      default:
        return 'Verification In Progress';
    }
  }

  String _getOverallStatusDescription(String status) {
    switch (status) {
      case 'verified':
        return 'Congratulations! Your profile has been reviewed and approved. You can now access your dashboard.';
      case 'pending_review':
        return 'We have received your verification documents. Our administrators are currently reviewing them. You will be notified shortly.';
      case 'rejected':
        return 'Your verification was rejected by the admin. Please verify your details or re-upload clear photos.';
      case 'suspended':
        return 'Your account is suspended. Please reach out to customer support for more information.';
      case 'in_progress':
      default:
        return 'Please complete all onboarding and verification steps to continue.';
    }
  }
}

class _StepListTile extends StatelessWidget {
  const _StepListTile({required this.step});
  final VerificationStepSummary step;

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 14),
      child: Row(
        children: [
          _getStepIcon(step.status),
          const SizedBox(width: 14),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  _getStepNameLabel(step.step),
                  style: const TextStyle(
                    fontSize: 14,
                    fontWeight: FontWeight.w700,
                    color: Color(0xFF1A1A1A),
                  ),
                ),
                if (step.isOptional) ...[
                  const SizedBox(height: 2),
                  const Text(
                    'Optional Step',
                    style: TextStyle(
                      fontSize: 11,
                      color: Color(0xFF888888),
                    ),
                  ),
                ],
              ],
            ),
          ),
          _getStatusBadge(step.status),
        ],
      ),
    );
  }

  String _getStepNameLabel(String step) {
    switch (step) {
      case 'identity':
        return 'Identity Verification';
      case 'licence':
        return 'Driver\'s License';
      case 'face':
        return 'Facial Matching';
      case 'vehicle':
        return 'Vehicle Verification';
      case 'guarantor':
        return 'Guarantor Setup';
      case 'emergency':
        return 'Emergency Contact';
      default:
        return step.toUpperCase();
    }
  }

  Widget _getStepIcon(String status) {
    switch (status) {
      case 'approved':
        return const Icon(Icons.check_circle_rounded, color: Color(0xFF4CAF50), size: 24);
      case 'submitted':
        return const Icon(Icons.watch_later_rounded, color: Color(0xFFF57F17), size: 24);
      case 'rejected':
        return const Icon(Icons.cancel_rounded, color: Color(0xFFC62828), size: 24);
      case 'pending':
      default:
        return const Icon(Icons.radio_button_unchecked_rounded, color: Color(0xFFBBBBBB), size: 24);
    }
  }

  Widget _getStatusBadge(String status) {
    Color bgColor;
    Color textColor;
    String label;

    switch (status) {
      case 'approved':
        bgColor = const Color(0xFFE8F5E9);
        textColor = const Color(0xFF2E7D32);
        label = 'Approved';
        break;
      case 'submitted':
        bgColor = const Color(0xFFFFF8E1);
        textColor = const Color(0xFFF57F17);
        label = 'Submitted';
        break;
      case 'rejected':
        bgColor = const Color(0xFFFFEBEE);
        textColor = const Color(0xFFC62828);
        label = 'Rejected';
        break;
      case 'pending':
      default:
        bgColor = const Color(0xFFF5F5F5);
        textColor = const Color(0xFF757575);
        label = 'Pending';
        break;
    }

    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 4),
      decoration: BoxDecoration(
        color: bgColor,
        borderRadius: BorderRadius.circular(6),
      ),
      child: Text(
        label,
        style: TextStyle(
          fontSize: 11,
          fontWeight: FontWeight.w700,
          color: textColor,
        ),
      ),
    );
  }
}