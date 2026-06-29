import 'package:flutter/material.dart';
import '../state/provider_profile_controller.dart';
import '../models/provider_profile_models.dart';

class GuarantorInformationScreen extends StatefulWidget {
  const GuarantorInformationScreen({
    super.key,
    required this.profileController,
  });

  final ProviderProfileController profileController;

  @override
  State<GuarantorInformationScreen> createState() =>
      _GuarantorInformationScreenState();
}

class _GuarantorInformationScreenState
    extends State<GuarantorInformationScreen> {
  bool _isLoading = false;
  bool _isSaving = false;
  String? _loadError;

  Guarantor? _saved;
  bool _isAdding = false;

  final _nameController = TextEditingController();
  final _phoneController = TextEditingController();

  bool get _canSave =>
      _nameController.text.trim().isNotEmpty &&
      _phoneController.text.trim().isNotEmpty &&
      !_isSaving;

  @override
  void initState() {
    super.initState();
    _loadExisting();
  }

  @override
  void dispose() {
    _nameController.dispose();
    _phoneController.dispose();
    super.dispose();
  }

  Future<void> _loadExisting() async {
    final cached = widget.profileController.guarantor;
    if (cached != null) {
      setState(() => _saved = cached);
      return;
    }

    setState(() {
      _isLoading = true;
      _loadError = null;
    });
    final result = await widget.profileController.loadGuarantor();
    if (!mounted) return;
    result.when(
      success: (g) => setState(() {
        _saved = g;
        _isLoading = false;
      }),
      failure: (error) => setState(() {
        _isLoading = false;
        if (error.message.toLowerCase().contains('not found') ||
            error.code == 'not_found') {
          _saved = null;
        } else {
          _loadError = error.message;
        }
      }),
    );
  }

  void _startAdding() {
    _nameController.text = _saved?.fullName ?? '';
    _phoneController.text = _saved != null
        ? _saved!.phone.replaceFirst(RegExp(r'^\+234'), '')
        : '';
    setState(() => _isAdding = true);
  }

  Future<void> _onSave() async {
    final phone = '+234${_phoneController.text.trim()}';
    setState(() => _isSaving = true);
    final result = await widget.profileController.saveGuarantor(
      fullName: _nameController.text.trim(),
      phone: phone,
    );
    if (!mounted) return;
    result.when(
      success: (g) {
        setState(() {
          _saved = g;
          _isAdding = false;
          _isSaving = false;
        });
        ScaffoldMessenger.of(context).showSnackBar(
          const SnackBar(content: Text('Guarantor information saved')),
        );
      },
      failure: (error) {
        setState(() => _isSaving = false);
        ScaffoldMessenger.of(
          context,
        ).showSnackBar(SnackBar(content: Text(error.message)));
      },
    );
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: Colors.white,
      body: SafeArea(
        child: _isLoading
            ? const Center(
                child: CircularProgressIndicator(color: Color(0xFF4CAF50)),
              )
            : Column(
                children: [
                  Expanded(
                    child: ListView(
                      padding: const EdgeInsets.fromLTRB(20, 12, 20, 24),
                      children: [
                        // ── Header ──────────────────────────────────
                        GestureDetector(
                          behavior: HitTestBehavior.opaque,
                          onTap: () => Navigator.of(context).pop(),
                          child: const Align(
                            alignment: Alignment.centerLeft,
                            child: Icon(
                              Icons.arrow_back_ios_new,
                              size: 20,
                              color: Color(0xFF1A1A1A),
                            ),
                          ),
                        ),
                        const SizedBox(height: 16),
                        const Text(
                          'Guarantor Information',
                          style: TextStyle(
                            fontSize: 20,
                            fontWeight: FontWeight.w800,
                            color: Color(0xFF1A1A1A),
                          ),
                        ),
                        const SizedBox(height: 2),
                        const Text(
                          'Provide information about who we can contact in case of an emergency.',
                          style: TextStyle(
                            fontSize: 13,
                            color: Color(0xFF888888),
                          ),
                        ),

                        const SizedBox(height: 16),

                        if (_loadError != null) ...[
                          Text(
                            _loadError!,
                            style: const TextStyle(
                              color: Color(0xFFE53935),
                              fontSize: 13,
                            ),
                          ),
                          const SizedBox(height: 12),
                          OutlinedButton(
                            onPressed: _loadExisting,
                            child: const Text('Retry'),
                          ),
                        ] else if (_saved != null && !_isAdding) ...[
                          Row(
                            mainAxisAlignment: MainAxisAlignment.end,
                            children: [
                              _AddNewButton(
                                label: 'Edit',
                                onTap: _startAdding,
                              ),
                            ],
                          ),
                          const SizedBox(height: 12),
                          _GuarantorCard(guarantor: _saved!),
                        ] else ...[
                          const Text(
                            'We need you to provide guarantor information. This could be a family member or close relative.',
                            style: TextStyle(
                              fontSize: 13,
                              color: Color(0xFF4CAF50),
                              height: 1.5,
                            ),
                          ),
                          const SizedBox(height: 20),
                          const Text(
                            'Guarantor Information',
                            style: TextStyle(
                              fontSize: 14,
                              fontWeight: FontWeight.w700,
                              color: Color(0xFF1A1A1A),
                            ),
                          ),
                          const SizedBox(height: 16),
                          const _FieldLabel('Full Name'),
                          const SizedBox(height: 8),
                          _TextInput(
                            controller: _nameController,
                            hint: 'Enter name',
                            onChanged: (_) => setState(() {}),
                          ),
                          const SizedBox(height: 16),
                          const _FieldLabel('Mobile Number'),
                          const SizedBox(height: 8),
                          _PhoneInput(
                            controller: _phoneController,
                            onChanged: (_) => setState(() {}),
                          ),
                        ],
                      ],
                    ),
                  ),

                  // ── Save button ──────────────────────────────────────
                  if (_saved == null || _isAdding)
                    Padding(
                      padding: const EdgeInsets.fromLTRB(20, 0, 20, 16),
                      child: SizedBox(
                        height: 52,
                        width: double.infinity,
                        child: FilledButton(
                          onPressed: _canSave ? _onSave : null,
                          style: FilledButton.styleFrom(
                            backgroundColor: const Color(0xFF4CAF50),
                            disabledBackgroundColor: const Color(
                              0xFF4CAF50,
                            ).withValues(alpha: 0.35),
                            shape: RoundedRectangleBorder(
                              borderRadius: BorderRadius.circular(999),
                            ),
                          ),
                          child: _isSaving
                              ? const SizedBox(
                                  width: 20,
                                  height: 20,
                                  child: CircularProgressIndicator(
                                    strokeWidth: 2,
                                    color: Colors.white,
                                  ),
                                )
                              : const Text(
                                  'Save',
                                  style: TextStyle(
                                    fontSize: 16,
                                    fontWeight: FontWeight.w700,
                                    color: Colors.white,
                                  ),
                                ),
                        ),
                      ),
                    ),
                ],
              ),
      ),
    );
  }
}

class _GuarantorCard extends StatelessWidget {
  const _GuarantorCard({required this.guarantor});
  final Guarantor guarantor;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 14),
      decoration: BoxDecoration(
        color: Colors.white,
        border: Border.all(color: const Color(0xFFEEEEEE)),
        borderRadius: BorderRadius.circular(12),
      ),
      child: Row(
        children: [
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  guarantor.fullName,
                  style: const TextStyle(
                    fontSize: 14,
                    fontWeight: FontWeight.w700,
                    color: Color(0xFF1A1A1A),
                  ),
                ),
                const SizedBox(height: 2),
                Text(
                  guarantor.phone,
                  style: const TextStyle(
                    fontSize: 13,
                    color: Color(0xFF888888),
                  ),
                ),
              ],
            ),
          ),
          const Icon(Icons.chevron_right, size: 20, color: Color(0xFF888888)),
        ],
      ),
    );
  }
}

class _AddNewButton extends StatelessWidget {
  const _AddNewButton({required this.onTap, this.label = '+ Add New'});
  final VoidCallback onTap;
  final String label;

  @override
  Widget build(BuildContext context) {
    return GestureDetector(
      onTap: onTap,
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 8),
        decoration: BoxDecoration(
          color: const Color(0xFF4CAF50),
          borderRadius: BorderRadius.circular(999),
        ),
        child: Text(
          label,
          style: const TextStyle(
            fontSize: 13,
            fontWeight: FontWeight.w700,
            color: Colors.white,
          ),
        ),
      ),
    );
  }
}

class _FieldLabel extends StatelessWidget {
  const _FieldLabel(this.text);
  final String text;

  @override
  Widget build(BuildContext context) {
    return Text(
      text,
      style: const TextStyle(
        fontSize: 13,
        fontWeight: FontWeight.w600,
        color: Color(0xFF1A1A1A),
      ),
    );
  }
}

class _TextInput extends StatelessWidget {
  const _TextInput({
    required this.controller,
    this.hint,
    required this.onChanged,
  });
  final TextEditingController controller;
  final String? hint;
  final ValueChanged<String> onChanged;

  @override
  Widget build(BuildContext context) {
    return TextField(
      controller: controller,
      onChanged: onChanged,
      style: const TextStyle(
        fontSize: 14,
        fontWeight: FontWeight.w500,
        color: Color(0xFF1A1A1A),
      ),
      decoration: InputDecoration(
        hintText: hint,
        hintStyle: const TextStyle(color: Color(0xFFAAAAAA), fontSize: 14),
        filled: true,
        fillColor: const Color(0xFFF5F6F8),
        contentPadding: const EdgeInsets.symmetric(
          horizontal: 16,
          vertical: 14,
        ),
        border: OutlineInputBorder(
          borderRadius: BorderRadius.circular(12),
          borderSide: BorderSide.none,
        ),
      ),
    );
  }
}

class _PhoneInput extends StatelessWidget {
  const _PhoneInput({required this.controller, required this.onChanged});
  final TextEditingController controller;
  final ValueChanged<String> onChanged;

  @override
  Widget build(BuildContext context) {
    return Container(
      decoration: BoxDecoration(
        color: const Color(0xFFF5F6F8),
        borderRadius: BorderRadius.circular(12),
      ),
      child: Row(
        children: [
          Padding(
            padding: const EdgeInsets.symmetric(horizontal: 12),
            child: Row(
              children: const [
                Text('🇳🇬', style: TextStyle(fontSize: 18)),
                SizedBox(width: 4),
                Icon(
                  Icons.keyboard_arrow_down,
                  size: 16,
                  color: Color(0xFF888888),
                ),
                SizedBox(width: 6),
                Text(
                  '(+234)',
                  style: TextStyle(
                    fontSize: 13,
                    color: Color(0xFF888888),
                    fontWeight: FontWeight.w500,
                  ),
                ),
              ],
            ),
          ),
          Container(width: 1, height: 20, color: const Color(0xFFDDDDDD)),
          Expanded(
            child: TextField(
              controller: controller,
              onChanged: onChanged,
              keyboardType: TextInputType.phone,
              style: const TextStyle(
                fontSize: 14,
                fontWeight: FontWeight.w500,
                color: Color(0xFF1A1A1A),
              ),
              decoration: const InputDecoration(
                hintText: 'Enter phone number',
                hintStyle: TextStyle(color: Color(0xFFAAAAAA), fontSize: 14),
                border: InputBorder.none,
                contentPadding: EdgeInsets.symmetric(
                  horizontal: 12,
                  vertical: 14,
                ),
              ),
            ),
          ),
        ],
      ),
    );
  }
}
