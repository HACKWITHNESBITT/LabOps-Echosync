import 'dart:async';

import 'package:flutter/material.dart';

import 'models/echosync_models.dart';
import 'services/echosync_client.dart';

class IcebreakerScreen extends StatefulWidget {
  final EchoMatch? match;
  final EchoSyncClient? client;

  const IcebreakerScreen({super.key, this.match, this.client});

  @override
  State<IcebreakerScreen> createState() => _IcebreakerScreenState();
}

class _IcebreakerScreenState extends State<IcebreakerScreen> {
  EchoMatch? _match;
  int _remaining = 0;
  String _icebreaker = '';
  bool _loadingIcebreaker = false;
  Timer? _ticker;

  @override
  void initState() {
    super.initState();
    _applyMatch(widget.match);
  }

  @override
  void didUpdateWidget(covariant IcebreakerScreen oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (widget.match?.id != oldWidget.match?.id) {
      _applyMatch(widget.match);
    }
  }

  void _applyMatch(EchoMatch? match) {
    _ticker?.cancel();
    _match = match;
    if (match != null) {
      _icebreaker = match.icebreaker;
      _remaining = match.remainingSeconds.clampInt(0);
      _ticker = Timer.periodic(const Duration(seconds: 1), (_) {
        if (!mounted) return;
        setState(() {
          _remaining = _match?.remainingSeconds.clampInt(0) ?? 0;
        });
      });
    } else {
      _icebreaker = '';
      _remaining = 0;
    }
  }

  Future<void> _regenerateIcebreaker() async {
    final match = _match;
    if (match == null) return;

    setState(() => _loadingIcebreaker = true);
    try {
      final client = widget.client ?? EchoSyncClient(userId: match.peerUserId);
      final text = await client.fetchIcebreaker(
        matchId: match.id,
        forUser: match.peerUserId,
      );
      if (text.isNotEmpty && mounted) setState(() => _icebreaker = text);
    } catch (_) {
      // network/edge offline: keep the last known icebreaker
    } finally {
      if (mounted) setState(() => _loadingIcebreaker = false);
    }
  }

  @override
  void dispose() {
    _ticker?.cancel();
    super.dispose();
  }

  String get _countdownLabel {
    if (_match == null) return '--:--';
    final s = _remaining;
    final m = s ~/ 60;
    final sec = s % 60;
    return '${m.toString().padLeft(2, '0')}:${sec.toString().padLeft(2, '0')}';
  }

  @override
  Widget build(BuildContext context) {
    final match = _match;

    return Container(
      decoration: const BoxDecoration(
        gradient: LinearGradient(
          begin: Alignment.topCenter,
          end: Alignment.bottomCenter,
          colors: [
            Color(0xFF121A29),
            Color(0xFF17243A),
            Color(0xFF17243A),
          ],
        ),
      ),
      child: SafeArea(
        child: Column(
          children: [
            // Pink line
            Container(
              height: 2,
              width: double.infinity,
              color: const Color(0xFFE7839A),
            ),

            // Header
            Container(
              height: 62,
              padding: const EdgeInsets.symmetric(horizontal: 24),
              decoration: const BoxDecoration(
                border: Border(
                  bottom: BorderSide(color: Color(0xFF657187), width: 1),
                ),
              ),
              child: Row(
                mainAxisAlignment: MainAxisAlignment.spaceBetween,
                children: [
                  const Text(
                    'EchoSync',
                    style: TextStyle(
                      fontSize: 19,
                      fontWeight: FontWeight.bold,
                      color: Colors.white,
                    ),
                  ),
                  Text(
                    _countdownLabel,
                    style: const TextStyle(
                      fontSize: 16,
                      fontWeight: FontWeight.w600,
                      color: Color(0xFFE0E5ED),
                    ),
                  ),
                ],
              ),
            ),

            // Content
            Expanded(
              child: match == null
                  ? _buildEmptyState()
                  : _buildMatchContent(match),
            ),
          ],
        ),
      ),
    );
  }

  Widget _buildEmptyState() {
    return Padding(
      padding: const EdgeInsets.symmetric(horizontal: 32),
      child: Column(
        mainAxisAlignment: MainAxisAlignment.center,
        children: [
          Container(
            width: 72,
            height: 72,
            decoration: BoxDecoration(
              shape: BoxShape.circle,
              color: const Color(0xFF1E2D44),
              border: Border.all(color: const Color(0xFF384D6B), width: 1.5),
            ),
            child: const Icon(
              Icons.sensors,
              size: 34,
              color: Color(0xFFE7839A),
            ),
          ),
          const SizedBox(height: 24),
          const Text(
            'No Active Match Nearby',
            style: TextStyle(
              fontSize: 18,
              fontWeight: FontWeight.bold,
              color: Colors.white,
            ),
          ),
          const SizedBox(height: 10),
          const Text(
            'Move within 15 meters of a peer with shared interest vectors to generate a dynamic conversation starter.',
            textAlign: TextAlign.center,
            style: TextStyle(
              fontSize: 13,
              color: Color(0xFF9BAAC1),
              height: 1.5,
            ),
          ),
          const SizedBox(height: 28),
          Container(
            padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 10),
            decoration: BoxDecoration(
              color: const Color(0xFF162338),
              borderRadius: BorderRadius.circular(8),
              border: Border.all(color: const Color(0xFF273C58)),
            ),
            child: const Row(
              mainAxisSize: MainAxisSize.min,
              children: [
                SizedBox(
                  width: 8,
                  height: 8,
                  child: CircularProgressIndicator(
                    strokeWidth: 1.5,
                    color: Color(0xFF2FE6A7),
                  ),
                ),
                SizedBox(width: 10),
                Text(
                  'Radar scanning for peers…',
                  style: TextStyle(
                    fontSize: 12,
                    fontFamily: 'monospace',
                    color: Color(0xFFD0D7E1),
                  ),
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }

  Widget _buildMatchContent(EchoMatch match) {
    final shared = match.sharedTokens.isNotEmpty
        ? match.sharedTokens.join(' · ')
        : 'shared interest';

    return Padding(
      padding: const EdgeInsets.fromLTRB(28, 36, 28, 20),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          const Text(
            'shared interest',
            style: TextStyle(fontSize: 13, color: Color(0xFFB5C0D0)),
          ),
          const SizedBox(height: 14),
          Text(
            shared,
            style: const TextStyle(
              fontSize: 28,
              fontWeight: FontWeight.w400,
              letterSpacing: -0.5,
              color: Colors.white,
            ),
          ),
          const SizedBox(height: 14),
          Row(
            children: [
              Expanded(
                child: Container(height: 1, color: const Color(0xFFB3BDCD)),
              ),
              const SizedBox(width: 14),
              Text(
                'similarity ${match.similarity.toStringAsFixed(3)}',
                style: const TextStyle(
                  fontSize: 12,
                  color: Color(0xFFD0D7E1),
                ),
              ),
            ],
          ),
          const SizedBox(height: 48),
          const Row(
            children: [
              Text(
                'icebreaker · on-device',
                style: TextStyle(fontSize: 13, color: Color(0xFFB5C0D0)),
              ),
            ],
          ),
          const SizedBox(height: 28),
          GestureDetector(
            onTap: _loadingIcebreaker ? null : _regenerateIcebreaker,
            child: Text(
              _loadingIcebreaker
                  ? 'generating…'
                  : (_icebreaker.isNotEmpty ? _icebreaker : 'Tap to generate icebreaker'),
              style: const TextStyle(
                fontSize: 18,
                height: 1.55,
                fontWeight: FontWeight.w600,
                color: Color(0xFFE8EBEF),
              ),
            ),
          ),
          const Spacer(),
          Container(height: 1, color: const Color(0xFF788598)),
          const SizedBox(height: 16),
          SizedBox(
            width: double.infinity,
            height: 52,
            child: OutlinedButton(
              onPressed: () {
                ScaffoldMessenger.of(context).showSnackBar(
                  SnackBar(content: Text('Hello sent to ${match.peerUserId} 👋')),
                );
              },
              style: OutlinedButton.styleFrom(
                side: const BorderSide(color: Color(0xFFD0D7E1)),
                shape: const RoundedRectangleBorder(
                  borderRadius: BorderRadius.zero,
                ),
              ),
              child: const Text(
                'say hello',
                style: TextStyle(
                  fontSize: 14,
                  fontWeight: FontWeight.w600,
                  color: Colors.white,
                ),
              ),
            ),
          ),
          const SizedBox(height: 12),
          Center(
            child: Text(
              'connection expires in $_countdownLabel',
              style: const TextStyle(
                fontSize: 10,
                color: Color(0xFF7E899A),
              ),
            ),
          ),
        ],
      ),
    );
  }
}

extension IntClamp on int {
  int clampInt(int low) => this < low ? low : this;
}