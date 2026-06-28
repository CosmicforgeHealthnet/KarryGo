import 'package:flutter/foundation.dart';

/// A support complaint / dispute (support-dispute-service).
@immutable
class Complaint {
  const Complaint({
    required this.id,
    required this.serviceType,
    required this.bookingReference,
    required this.subject,
    required this.description,
    required this.status,
    required this.createdAt,
    this.resolvedAt,
  });

  final String id;
  final String serviceType;
  final String bookingReference;
  final String subject;
  final String description;
  final String status;
  final DateTime createdAt;
  final DateTime? resolvedAt;

  /// 0 = Submitted, 1 = Processing, 2 = Completed — drives the status timeline.
  int get stage {
    switch (status) {
      case 'resolved':
      case 'closed':
        return 2;
      case 'under_review':
      case 'awaiting_evidence':
      case 'escalated':
        return 1;
      default:
        return 0;
    }
  }

  /// Friendly status label for the feedback list.
  String get statusLabel {
    switch (status) {
      case 'resolved':
      case 'closed':
        return 'Completed';
      case 'under_review':
      case 'awaiting_evidence':
      case 'escalated':
        return 'Processing';
      default:
        return 'Pending';
    }
  }

  factory Complaint.fromJson(Map<String, dynamic> j) => Complaint(
        id: j['id'] as String? ?? '',
        serviceType: j['service_type'] as String? ?? '',
        bookingReference: j['booking_reference'] as String? ?? '',
        subject: j['subject'] as String? ?? '',
        description: j['description'] as String? ?? '',
        status: j['status'] as String? ?? 'open',
        createdAt: DateTime.tryParse(j['created_at'] as String? ?? '')?.toLocal() ??
            DateTime.fromMillisecondsSinceEpoch(0),
        resolvedAt: j['resolved_at'] != null
            ? DateTime.tryParse(j['resolved_at'] as String)?.toLocal()
            : null,
      );
}

/// A live-chat message on a complaint.
@immutable
class DisputeMessage {
  const DisputeMessage({
    required this.id,
    required this.senderType,
    required this.content,
    required this.createdAt,
  });

  final String id;
  final String senderType;
  final String content;
  final DateTime createdAt;

  /// True when sent by this provider (right-aligned bubble).
  bool get isMine => senderType == 'hauling_provider';

  factory DisputeMessage.fromJson(Map<String, dynamic> j) => DisputeMessage(
        id: j['id'] as String? ?? '',
        senderType: j['sender_type'] as String? ?? '',
        content: j['content'] as String? ?? '',
        createdAt: DateTime.tryParse(j['created_at'] as String? ?? '')?.toLocal() ??
            DateTime.fromMillisecondsSinceEpoch(0),
      );
}

/// The predefined dispute types shown on the "Select Dispute Type" screen.
const disputeTypes = <String>[
  'Successful withdrawal not credited to bank',
  'Cancelled trip',
  'Over deduction',
  'Pending transaction',
  'Failed transaction',
];
