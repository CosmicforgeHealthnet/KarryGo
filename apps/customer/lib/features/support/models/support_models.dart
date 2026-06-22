class ChatMessage {
  const ChatMessage({
    required this.id,
    required this.complaintId,
    required this.senderType,
    required this.senderId,
    required this.content,
    required this.isRead,
    required this.createdAt,
  });

  final String id;
  final String complaintId;
  final String senderType;
  final String senderId;
  final String content;
  final bool isRead;
  final DateTime createdAt;

  bool get isAdmin => senderType == 'admin';

  factory ChatMessage.fromJson(Map<String, dynamic> json) {
    return ChatMessage(
      id: json['id']?.toString() ?? '',
      complaintId: json['complaint_id']?.toString() ?? '',
      senderType: json['sender_type']?.toString() ?? '',
      senderId: json['sender_id']?.toString() ?? '',
      content: json['content']?.toString() ?? '',
      isRead: json['is_read'] == true,
      createdAt: DateTime.tryParse(json['created_at']?.toString() ?? '') ?? DateTime.now(),
    );
  }
}

class Complaint {
  const Complaint({
    required this.id,
    required this.serviceType,
    required this.subject,
    required this.description,
    required this.status,
    this.bookingReference,
    this.resolutionNote,
    this.resolvedAt,
    required this.createdAt,
    required this.updatedAt,
  });

  final String id;
  final String serviceType;
  final String subject;
  final String description;
  final String status;
  final String? bookingReference;
  final String? resolutionNote;
  final DateTime? resolvedAt;
  final DateTime createdAt;
  final DateTime updatedAt;

  factory Complaint.fromJson(Map<String, dynamic> json) {
    return Complaint(
      id: json['id']?.toString() ?? '',
      serviceType: json['service_type']?.toString() ?? '',
      subject: json['subject']?.toString() ?? '',
      description: json['description']?.toString() ?? '',
      status: json['status']?.toString() ?? 'open',
      bookingReference: json['booking_reference']?.toString(),
      resolutionNote: json['resolution_note']?.toString(),
      resolvedAt: json['resolved_at'] != null
          ? DateTime.tryParse(json['resolved_at'].toString())
          : null,
      createdAt: DateTime.tryParse(json['created_at']?.toString() ?? '') ??
          DateTime.now(),
      updatedAt: DateTime.tryParse(json['updated_at']?.toString() ?? '') ??
          DateTime.now(),
    );
  }

  String get statusLabel => switch (status) {
        'open' => 'Open',
        'under_review' => 'Under Review',
        'awaiting_evidence' => 'Awaiting Evidence',
        'resolved' => 'Resolved',
        'closed' => 'Closed',
        'escalated' => 'Escalated',
        _ => status,
      };

  bool get isClosed =>
      status == 'resolved' || status == 'closed';
}
