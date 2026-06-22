import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../../../shared/widgets/figma_customer_widgets.dart';
import '../../auth/data/customer_auth_api.dart';
import '../../auth/models/customer_auth_models.dart';

class EmergencyContactScreen extends StatefulWidget {
  const EmergencyContactScreen({
    super.key,
    required this.session,
    required this.api,
  });

  final CustomerSession session;
  final CustomerAuthApi api;

  @override
  State<EmergencyContactScreen> createState() => _EmergencyContactScreenState();
}

class _EmergencyContactScreenState extends State<EmergencyContactScreen> {
  List<EmergencyContact> _contacts = [];
  bool _loading = true;
  ApiException? _error;

  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load() async {
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      final contacts = await widget.api.getEmergencyContacts(
        accessToken: widget.session.accessToken,
      );
      setState(() {
        _contacts = contacts;
        _loading = false;
      });
    } on ApiException catch (e) {
      setState(() {
        _error = e;
        _loading = false;
      });
    }
  }

  Future<void> _delete(EmergencyContact contact) async {
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: const Text('Remove Contact'),
        content: Text('Remove ${contact.name} from emergency contacts?'),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(ctx).pop(false),
            child: const Text('Cancel'),
          ),
          TextButton(
            onPressed: () => Navigator.of(ctx).pop(true),
            child: const Text('Remove', style: TextStyle(color: Color(0xFFE53935))),
          ),
        ],
      ),
    );
    if (confirmed != true) return;

    try {
      await widget.api.deleteEmergencyContact(
        accessToken: widget.session.accessToken,
        id: contact.id,
      );
      setState(() => _contacts.removeWhere((c) => c.id == contact.id));
    } on ApiException catch (e) {
      if (!mounted) return;
      ScaffoldMessenger.of(context)
          .showSnackBar(SnackBar(content: Text(e.message)));
    }
  }

  Future<void> _openForm() async {
    final added = await Navigator.of(context).push<EmergencyContact>(
      MaterialPageRoute(
        builder: (_) => _EmergencyContactFormScreen(
          session: widget.session,
          api: widget.api,
        ),
      ),
    );
    if (added != null) {
      setState(() => _contacts.add(added));
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: CustomerFigmaColors.surface,
      appBar: AppBar(
        backgroundColor: Colors.white,
        elevation: 0,
        leading: IconButton(
          icon: const Icon(Icons.arrow_back_rounded, color: CustomerFigmaColors.text),
          onPressed: () => Navigator.of(context).pop(),
        ),
        title: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: const [
            Text(
              'Emergency Contact',
              style: TextStyle(
                color: CustomerFigmaColors.text,
                fontSize: 17,
                fontWeight: FontWeight.w800,
              ),
            ),
            Text(
              'Provide info about who we can contact.',
              style: TextStyle(color: CustomerFigmaColors.muted, fontSize: 11),
            ),
          ],
        ),
        actions: [
          Padding(
            padding: const EdgeInsets.only(right: 12),
            child: ElevatedButton.icon(
              onPressed: _openForm,
              icon: const Icon(Icons.add_rounded, size: 16),
              label: const Text(
                '+ Add New',
                style: TextStyle(fontSize: 13, fontWeight: FontWeight.w700),
              ),
              style: ElevatedButton.styleFrom(
                backgroundColor: CustomerFigmaColors.primary,
                foregroundColor: Colors.white,
                elevation: 0,
                shape: RoundedRectangleBorder(
                    borderRadius: BorderRadius.circular(20)),
                padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 8),
              ),
            ),
          ),
        ],
      ),
      body: _loading
          ? const Center(
              child: CircularProgressIndicator(color: CustomerFigmaColors.primary))
          : _error != null
              ? _ErrorView(error: _error!, onRetry: _load)
              : _contacts.isEmpty
                  ? const _EmptyView()
                  : ListView.separated(
                      padding: const EdgeInsets.all(16),
                      itemCount: _contacts.length,
                      separatorBuilder: (_, __) => const SizedBox(height: 10),
                      itemBuilder: (_, index) {
                        final c = _contacts[index];
                        return _ContactTile(
                          contact: c,
                          onDelete: () => _delete(c),
                        );
                      },
                    ),
    );
  }
}

// ─── Contact tile ─────────────────────────────────────────────────────────────

class _ContactTile extends StatelessWidget {
  const _ContactTile({required this.contact, required this.onDelete});
  final EmergencyContact contact;
  final VoidCallback onDelete;

  @override
  Widget build(BuildContext context) {
    return Container(
      decoration: BoxDecoration(
        color: Colors.white,
        borderRadius: BorderRadius.circular(12),
      ),
      child: ListTile(
        contentPadding: const EdgeInsets.symmetric(horizontal: 16, vertical: 8),
        title: Text(
          contact.name,
          style: const TextStyle(
            color: CustomerFigmaColors.text,
            fontWeight: FontWeight.w700,
            fontSize: 15,
          ),
        ),
        subtitle: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            const SizedBox(height: 2),
            Text(
              _formatPhone(contact.phone),
              style: const TextStyle(color: CustomerFigmaColors.muted, fontSize: 13),
            ),
            Text(
              contact.relationship,
              style: const TextStyle(color: CustomerFigmaColors.muted, fontSize: 12),
            ),
          ],
        ),
        trailing: Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            const Icon(Icons.chevron_right_rounded, color: CustomerFigmaColors.muted),
          ],
        ),
        onLongPress: onDelete,
      ),
    );
  }

  String _formatPhone(String phone) {
    if (phone.startsWith('+234') && phone.length > 4) {
      return '🇳🇬 (+234) ${phone.substring(4)}';
    }
    return phone;
  }
}

// ─── Form screen ──────────────────────────────────────────────────────────────

class _EmergencyContactFormScreen extends StatefulWidget {
  const _EmergencyContactFormScreen({required this.session, required this.api});
  final CustomerSession session;
  final CustomerAuthApi api;

  @override
  State<_EmergencyContactFormScreen> createState() =>
      _EmergencyContactFormScreenState();
}

class _EmergencyContactFormScreenState
    extends State<_EmergencyContactFormScreen> {
  final _nameCtrl = TextEditingController();
  final _phoneCtrl = TextEditingController();
  String? _relationship;
  bool _saving = false;
  ApiException? _error;

  static const _relationships = [
    'Mother', 'Father', 'Sibling', 'Spouse', 'Friend', 'Other',
  ];

  bool get _canSave =>
      _nameCtrl.text.trim().isNotEmpty &&
      _phoneCtrl.text.trim().isNotEmpty &&
      _relationship != null;

  @override
  void initState() {
    super.initState();
    _nameCtrl.addListener(() => setState(() {}));
    _phoneCtrl.addListener(() => setState(() {}));
  }

  @override
  void dispose() {
    _nameCtrl.dispose();
    _phoneCtrl.dispose();
    super.dispose();
  }

  Future<void> _save() async {
    setState(() {
      _saving = true;
      _error = null;
    });
    try {
      final digits = _phoneCtrl.text.trim().replaceAll(RegExp(r'\D'), '');
      final phone = digits.startsWith('234')
          ? '+$digits'
          : digits.startsWith('0')
              ? '+234${digits.substring(1)}'
              : '+234$digits';

      final contact = await widget.api.addEmergencyContact(
        accessToken: widget.session.accessToken,
        name: _nameCtrl.text.trim(),
        phone: phone,
        relationship: _relationship!,
      );
      if (mounted) Navigator.of(context).pop(contact);
    } on ApiException catch (e) {
      setState(() {
        _error = e;
        _saving = false;
      });
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: CustomerFigmaColors.surface,
      appBar: AppBar(
        backgroundColor: Colors.white,
        elevation: 0,
        leading: IconButton(
          icon: const Icon(Icons.arrow_back_rounded, color: CustomerFigmaColors.text),
          onPressed: () => Navigator.of(context).pop(),
        ),
        title: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: const [
            Text(
              'Emergency Contact',
              style: TextStyle(
                color: CustomerFigmaColors.text,
                fontSize: 17,
                fontWeight: FontWeight.w800,
              ),
            ),
            Text(
              'Provide info about who we can contact.',
              style: TextStyle(color: CustomerFigmaColors.muted, fontSize: 11),
            ),
          ],
        ),
      ),
      body: Column(
        children: [
          Expanded(
            child: ListView(
              padding: const EdgeInsets.all(24),
              children: [
                const Text(
                  'Emergency Contact',
                  style: TextStyle(
                    color: CustomerFigmaColors.text,
                    fontWeight: FontWeight.w800,
                    fontSize: 15,
                  ),
                ),
                const SizedBox(height: 20),
                if (_error != null) ...[
                  Container(
                    padding: const EdgeInsets.all(12),
                    decoration: BoxDecoration(
                      color: const Color(0xFFFFF1F0),
                      borderRadius: BorderRadius.circular(10),
                      border: Border.all(color: const Color(0xFFFFCDD2)),
                    ),
                    child: Text(
                      _error!.message,
                      style: const TextStyle(
                          color: Color(0xFFC0392B), fontSize: 13),
                    ),
                  ),
                  const SizedBox(height: 16),
                ],
                FigmaTextField(
                  controller: _nameCtrl,
                  label: 'Full Name',
                  hintText: 'Enter name',
                ),
                const SizedBox(height: 20),
                const Text(
                  'Mobile Number',
                  style: TextStyle(
                    color: CustomerFigmaColors.text,
                    fontSize: 13,
                    fontWeight: FontWeight.w800,
                  ),
                ),
                const SizedBox(height: 8),
                Container(
                  decoration: BoxDecoration(
                    color: Colors.white,
                    borderRadius: BorderRadius.circular(10),
                  ),
                  child: Row(
                    children: [
                      Container(
                        padding: const EdgeInsets.symmetric(
                            horizontal: 12, vertical: 14),
                        decoration: const BoxDecoration(
                          border: Border(
                            right: BorderSide(
                                color: CustomerFigmaColors.border, width: 1),
                          ),
                        ),
                        child: const Text(
                          '🇳🇬  (+234)',
                          style: TextStyle(
                            color: CustomerFigmaColors.text,
                            fontSize: 14,
                            fontWeight: FontWeight.w600,
                          ),
                        ),
                      ),
                      Expanded(
                        child: TextField(
                          controller: _phoneCtrl,
                          keyboardType: TextInputType.phone,
                          inputFormatters: [
                            FilteringTextInputFormatter.digitsOnly,
                          ],
                          decoration: const InputDecoration(
                            hintText: '8012345678',
                            hintStyle: TextStyle(color: CustomerFigmaColors.muted),
                            border: InputBorder.none,
                            contentPadding:
                                EdgeInsets.symmetric(horizontal: 12, vertical: 14),
                          ),
                        ),
                      ),
                    ],
                  ),
                ),
                const SizedBox(height: 20),
                const Text(
                  'Relationship Type',
                  style: TextStyle(
                    color: CustomerFigmaColors.text,
                    fontSize: 13,
                    fontWeight: FontWeight.w800,
                  ),
                ),
                const SizedBox(height: 8),
                Container(
                  decoration: BoxDecoration(
                    color: Colors.white,
                    borderRadius: BorderRadius.circular(10),
                  ),
                  padding: const EdgeInsets.symmetric(horizontal: 16),
                  child: DropdownButtonHideUnderline(
                    child: DropdownButton<String>(
                      isExpanded: true,
                      value: _relationship,
                      hint: const Text(
                        'Select Relationship Type',
                        style: TextStyle(color: CustomerFigmaColors.muted),
                      ),
                      icon: const Icon(Icons.keyboard_arrow_down_rounded,
                          color: CustomerFigmaColors.muted),
                      items: _relationships
                          .map((r) => DropdownMenuItem(value: r, child: Text(r)))
                          .toList(),
                      onChanged: (v) => setState(() => _relationship = v),
                    ),
                  ),
                ),
              ],
            ),
          ),
          Padding(
            padding: const EdgeInsets.fromLTRB(24, 0, 24, 24),
            child: FigmaPrimaryButton(
              label: 'Save',
              isLoading: _saving,
              onPressed: _canSave && !_saving ? _save : null,
            ),
          ),
        ],
      ),
    );
  }
}

// ─── Shared widgets ───────────────────────────────────────────────────────────

class _EmptyView extends StatelessWidget {
  const _EmptyView();

  @override
  Widget build(BuildContext context) {
    return Center(
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          Container(
            width: 72,
            height: 72,
            decoration: const BoxDecoration(
              color: CustomerFigmaColors.primaryPale,
              shape: BoxShape.circle,
            ),
            child: const Icon(Icons.emergency_outlined,
                size: 34, color: CustomerFigmaColors.primary),
          ),
          const SizedBox(height: 16),
          const Text(
            'No emergency contacts yet',
            style: TextStyle(
              color: CustomerFigmaColors.text,
              fontSize: 16,
              fontWeight: FontWeight.w800,
            ),
          ),
          const SizedBox(height: 8),
          const Text(
            'Add a contact who can be reached\nin case of an emergency.',
            textAlign: TextAlign.center,
            style: TextStyle(color: CustomerFigmaColors.muted, fontSize: 13),
          ),
        ],
      ),
    );
  }
}

class _ErrorView extends StatelessWidget {
  const _ErrorView({required this.error, required this.onRetry});
  final ApiException error;
  final VoidCallback onRetry;

  @override
  Widget build(BuildContext context) {
    return Center(
      child: Padding(
        padding: const EdgeInsets.all(24),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            const Icon(Icons.error_outline_rounded,
                color: Color(0xFFE53935), size: 40),
            const SizedBox(height: 16),
            Text(error.message,
                textAlign: TextAlign.center,
                style: const TextStyle(
                    color: CustomerFigmaColors.text, fontSize: 14)),
            const SizedBox(height: 20),
            FigmaPrimaryButton(label: 'Try again', onPressed: onRetry),
          ],
        ),
      ),
    );
  }
}
