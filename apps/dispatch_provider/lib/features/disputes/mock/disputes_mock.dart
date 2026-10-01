import '../models/dispute_models.dart';

/// Stateful mock for the support/disputes feature. Seeds 4 cases matching
/// Figma 03, and a starter chat thread matching Figma 09.
class DisputesMock {
  DisputesMock._();

  static final List<Complaint> _complaints = _seedComplaints();
  static final Map<String, List<DisputeMessage>> _messages = {};

  static List<Complaint> complaints() =>
      List<Complaint>.unmodifiable(_complaints);

  static Complaint? get(String id) {
    for (final c in _complaints) {
      if (c.id == id) return c;
    }
    return null;
  }

  static Complaint addComplaint({
    required String subject,
    required String description,
    required String bookingReference,
  }) {
    final now = DateTime.now();
    final id = 'KAR-${(70000 + _complaints.length * 37)}';
    final c = Complaint(
      id: id,
      serviceType: 'dispatch',
      bookingReference: bookingReference,
      subject: subject,
      description: description,
      status: 'open',
      createdAt: now,
    );
    _complaints.insert(0, c);
    return c;
  }

  /// Returns (or creates) the well-known support-chat complaint.
  static Complaint supportChatComplaint() {
    for (final c in _complaints) {
      if (c.subject == 'Live chat support' &&
          c.status != 'resolved' &&
          c.status != 'closed') {
        return c;
      }
    }
    final now = DateTime.now();
    final c = Complaint(
      id: 'KAR-CHAT',
      serviceType: 'dispatch',
      bookingReference: '',
      subject: 'Live chat support',
      description: 'Live chat conversation with support.',
      status: 'open',
      createdAt: now,
    );
    _complaints.insert(0, c);
    return c;
  }

  // ── Messages ───────────────────────────────────────────────────────────

  static List<DisputeMessage> messages(String complaintId) {
    return _messages[complaintId] ??
        (_messages[complaintId] = _seedMessages());
  }

  static DisputeMessage sendMessage(String complaintId, String content) {
    final msg = DisputeMessage(
      id: 'msg-${DateTime.now().microsecondsSinceEpoch}',
      senderType: 'dispatch_provider',
      content: content,
      createdAt: DateTime.now(),
    );
    final list = _messages.putIfAbsent(complaintId, _seedMessages);
    list.add(msg);
    return msg;
  }

  static void reset() {
    _complaints
      ..clear()
      ..addAll(_seedComplaints());
    _messages.clear();
  }

  // ── Seeds ──────────────────────────────────────────────────────────────

  static List<Complaint> _seedComplaints() {
    final now = DateTime.now();
    return [
      Complaint(
        id: 'KAR-78423',
        serviceType: 'dispatch',
        bookingReference: 'PRK-2048731',
        subject: 'Delivery issue',
        description: 'Customer marked as unavailable but I was at the address.',
        status: 'open',
        createdAt: now.subtract(const Duration(hours: 2)),
      ),
      Complaint(
        id: 'KAR-77102',
        serviceType: 'dispatch',
        bookingReference: 'PRK-2048658',
        subject: 'Payment enquiry',
        description: 'Awaiting payout for completed trip PRK-2048658.',
        status: 'under_review',
        createdAt: now.subtract(const Duration(days: 1, hours: 4)),
      ),
      Complaint(
        id: 'KAR-65641',
        serviceType: 'dispatch',
        bookingReference: 'PRK-2048450',
        subject: 'Account verification',
        description: 'BVN verification was slow to reflect.',
        status: 'resolved',
        createdAt: now.subtract(const Duration(days: 3, hours: 2)),
        resolvedAt: now.subtract(const Duration(days: 2, hours: 20)),
      ),
      Complaint(
        id: 'KAR-66219',
        serviceType: 'dispatch',
        bookingReference: 'PRK-2048390',
        subject: 'App issue',
        description: 'Notifications were delayed on Android.',
        status: 'resolved',
        createdAt: now.subtract(const Duration(days: 5)),
        resolvedAt: now.subtract(const Duration(days: 4, hours: 6)),
      ),
    ];
  }

  static List<DisputeMessage> _seedMessages() {
    final now = DateTime.now();
    return [
      DisputeMessage(
        id: 'm1',
        senderType: 'dispatch_provider',
        content: "Hi. I'm having an issue with my last payout. Can you help?",
        createdAt: now.subtract(const Duration(minutes: 18)),
      ),
      DisputeMessage(
        id: 'm2',
        senderType: 'admin',
        content:
            "Thanks for reaching out. How can we help you today?",
        createdAt: now.subtract(const Duration(minutes: 15)),
      ),
      DisputeMessage(
        id: 'm3',
        senderType: 'dispatch_provider',
        content:
            "I'm having an issue with the payout request I wasn't available.",
        createdAt: now.subtract(const Duration(minutes: 12)),
      ),
    ];
  }
}