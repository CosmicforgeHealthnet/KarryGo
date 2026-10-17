import 'package:flutter/material.dart';
import '../../disputes/models/dispute_models.dart';
import '../../disputes/state/rider_dispute_controller.dart';
import '../../disputes/ui/support/case_detail_screen.dart';

class DisputeDetailScreen extends StatelessWidget {
  const DisputeDetailScreen({
    super.key,
    required this.complaint,
    required this.disputeController,
  });
  final Complaint complaint;
  final RiderDisputeController disputeController;
  @override
  Widget build(BuildContext context) => CaseDetailScreen(
    complaint: complaint,
    disputeController: disputeController,
  );
}
