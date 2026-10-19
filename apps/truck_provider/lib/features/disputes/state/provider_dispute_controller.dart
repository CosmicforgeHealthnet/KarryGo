import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:flutter/foundation.dart';

import '../../earnings/models/earnings_models.dart';
import '../data/provider_support_api.dart';
import '../models/dispute_models.dart';

/// Drives the disputes flow: the feedback list, the log-dispute steps
/// (select transaction → select type → submit), and per-complaint live chat.
class ProviderDisputeController extends ChangeNotifier {
  ProviderDisputeController({
    required ProviderSupportApi api,
    required String? Function() accessToken,
  })  : _api = api,
        _accessToken = accessToken;

  final ProviderSupportApi _api;
  final String? Function() _accessToken;
  String? get _token => _accessToken();

  // ─── Feedback list ──────────────────────────────────────────────────────────
  List<Complaint> _complaints = const [];
  List<Complaint> get complaints => _complaints;
  bool _loading = false;
  bool get loading => _loading;
  String? _error;
  String? get error => _error;

  Future<void> loadComplaints() async {
    final token = _token;
    if (token == null) return;
    _loading = true;
    _error = null;
    notifyListeners();
    try {
      _complaints = await _api.listComplaints(accessToken: token);
    } catch (e) {
      _error = _msg(e);
    } finally {
      _loading = false;
      notifyListeners();
    }
  }

  // ─── New-dispute selection ──────────────────────────────────────────────────
  EarningsTransaction? _selectedTransaction;
  EarningsTransaction? get selectedTransaction => _selectedTransaction;
  String? _selectedType;
  String? get selectedType => _selectedType;

  void startNewDispute() {
    _selectedTransaction = null;
    _selectedType = null;
    _submitError = null;
    _createdComplaint = null;
    notifyListeners();
  }

  void selectTransaction(EarningsTransaction txn) {
    _selectedTransaction = txn;
    notifyListeners();
  }

  void selectType(String type) {
    _selectedType = type;
    notifyListeners();
  }

  // ─── Submission ─────────────────────────────────────────────────────────────
  bool _submitting = false;
  bool get submitting => _submitting;
  String? _submitError;
  String? get submitError => _submitError;
  Complaint? _createdComplaint;
  Complaint? get createdComplaint => _createdComplaint;

  Future<Complaint?> submitDispute() async {
    final token = _token;
    final txn = _selectedTransaction;
    final type = _selectedType;
    if (token == null || txn == null || type == null) return null;

    _submitting = true;
    _submitError = null;
    notifyListeners();
    try {
      final reference = txn.bookingId.isNotEmpty ? txn.bookingId : txn.id;
      final complaint = await _api.createComplaint(
        accessToken: token,
        subject: type,
        description: 'Dispute on transaction ${txn.id}: $type',
        bookingReference: reference,
      );
      _createdComplaint = complaint;
      _complaints = [complaint, ..._complaints];
      return complaint;
    } catch (e) {
      _submitError = _msg(e);
      return null;
    } finally {
      _submitting = false;
      notifyListeners();
    }
  }

  // ─── Support live chat (profile > Support) ─────────────────────────────────
  //
  // The live chat is a regular complaint on support-dispute-service with a
  // well-known subject, so agent replies arrive through the same admin surface
  // as dispute chats. One open support complaint is reused per provider.
  static const supportChatSubject = 'Live chat support';

  Complaint? _supportComplaint;
  Complaint? get supportComplaint => _supportComplaint;
  bool _openingSupportChat = false;
  bool get openingSupportChat => _openingSupportChat;
  String? _supportChatError;
  String? get supportChatError => _supportChatError;

  /// Finds the provider's open support-chat complaint or creates one.
  /// Returns null on failure (error in [supportChatError]).
  /// Driver help articles. Throws on failure so the screen can say so.
  Future<List<({String title, String body})>> loadFaqs() async {
    final token = _token;
    if (token == null) return const [];
    return _api.listFaqs(accessToken: token);
  }

  /// Sends an SOS to support. Returns null on success, or the error message.
  Future<String?> sendSos({required String description, double? lat, double? lng}) async {
    final token = _token;
    if (token == null) return 'Please sign in again.';
    try {
      await _api.sendSos(accessToken: token, description: description, lat: lat, lng: lng);
      return null;
    } catch (e) {
      return e is ApiException ? e.message : 'Could not reach support. Call 112 if you are in danger.';
    }
  }

  Future<Complaint?> openSupportChat() async {
    final token = _token;
    if (token == null) return null;
    _openingSupportChat = true;
    _supportChatError = null;
    notifyListeners();
    try {
      final complaints = await _api.listComplaints(accessToken: token);
      Complaint? existing;
      for (final c in complaints) {
        final closed = c.status == 'resolved' || c.status == 'closed';
        if (c.subject == supportChatSubject && !closed) {
          existing = c;
          break;
        }
      }
      _supportComplaint = existing ??
          await _api.createComplaint(
            accessToken: token,
            subject: supportChatSubject,
            description: 'Live chat conversation with support.',
            bookingReference: '',
          );
      return _supportComplaint;
    } catch (e) {
      _supportChatError = _msg(e);
      return null;
    } finally {
      _openingSupportChat = false;
      notifyListeners();
    }
  }

  /// Marks agent messages on a complaint as read. Failure-silent.
  Future<void> markMessagesRead(String complaintId) async {
    final token = _token;
    if (token == null) return;
    try {
      await _api.markMessagesRead(accessToken: token, id: complaintId);
    } catch (_) {}
  }

  // ─── Live chat ──────────────────────────────────────────────────────────────
  List<DisputeMessage> _messages = const [];
  List<DisputeMessage> get messages => _messages;
  bool _loadingMessages = false;
  bool get loadingMessages => _loadingMessages;
  bool _sending = false;
  bool get sending => _sending;

  Future<void> loadMessages(String complaintId) async {
    final token = _token;
    if (token == null) return;
    _loadingMessages = true;
    notifyListeners();
    try {
      _messages = await _api.listMessages(accessToken: token, id: complaintId);
    } catch (_) {
      // Leave existing messages; the chat screen still renders the composer.
    } finally {
      _loadingMessages = false;
      notifyListeners();
    }
  }

  Future<void> sendMessage(String complaintId, String content) async {
    final token = _token;
    if (token == null || content.trim().isEmpty) return;
    _sending = true;
    notifyListeners();
    try {
      final msg = await _api.sendMessage(accessToken: token, id: complaintId, content: content.trim());
      _messages = [..._messages, msg];
    } catch (_) {
    } finally {
      _sending = false;
      notifyListeners();
    }
  }

  String _msg(Object e) => e is ApiException ? e.message : e.toString();
}
