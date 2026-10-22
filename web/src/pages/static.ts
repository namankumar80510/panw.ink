// About, support and policy pages. Policy text is a plain-language DRAFT for
// local legal review (§11.4, §13.3), not a claim of compliance.
import m from 'mithril';
import { pageHeader } from '../components/ui';
import { config } from '../stores/config';
import { setTitle } from '../utils/title';

export function AboutPage(): m.Component {
  return {
    oninit: () => setTitle('About'),
    view: () => m('.page-text', [
      pageHeader(`About ${config.appName}`),
      m('p', `${config.appName} is an independent imprint publishing original, human-written fiction under several pen names.`),
      m('h2.h5.mt-4', 'Two formats, both first-class'),
      m('p', 'A short story is one complete text you can read in a sitting — never chopped into chapters. A novel is a sequence of chapters, sometimes grouped into volumes. Anthologies collect independent short stories.'),
      m('h2.h5.mt-4', 'Free eventual access'),
      m('p', 'Every chapter and story has a public release date. Patreon supporters may read some of them earlier; nothing is permanently locked, and there are no coins or per-chapter purchases.'),
      m('h2.h5.mt-4', 'Human authorship'),
      m('p', 'We do not publish mass-produced AI-generated narrative. Tools for editing, proofreading and research are fine; the story itself is written by people.'),
    ]),
  };
}

export function SupportPage(): m.Component {
  return {
    oninit: () => setTitle('Support'),
    view: () => m('.page-text', [
      pageHeader('Support the fiction'),
      m('p', 'All main fiction eventually releases free. Supporting us on Patreon provides early access while your membership remains entitled.'),
      m('ol', [
        m('li', 'Become a member on Patreon (payment happens on Patreon, never here).'),
        m('li', 'Create a reader account here and verify your email.'),
        m('li', 'Connect Patreon from Settings → Patreon. Early chapters unlock for your tier until their public date, when they become free for everyone.'),
      ]),
      config.value?.patreon_url
        ? m('a.btn.btn-primary', { href: config.value.patreon_url, rel: 'noopener', target: '_blank' }, 'Visit our Patreon')
        : m('p.text-body-secondary', 'Patreon linking is not enabled yet.'),
    ]),
  };
}

const POLICIES: Record<string, { title: string; body: string[] }> = {
  terms: {
    title: 'Terms of use',
    body: [
      'Reading public fiction requires no account. Accounts are for readers who want to save progress, review, comment or link Patreon.',
      'Authors keep the copyright in their works. Any future invited contributor grants only the minimal non-exclusive permission needed to host and display their work, and may withdraw it.',
      'Do not scrape, republish or redistribute works without the author’s permission.',
      'Draft wording pending legal review.',
    ],
  },
  privacy: {
    title: 'Privacy',
    body: [
      'No third-party advertising or tracking. Reading analytics are first-party, aggregated and used to improve the library and to rank fiction fairly.',
      'Anonymous reading is always allowed. Your email and Patreon identity are private; reviews and comments are public under your display name.',
      'Raw reading events are kept for at most 30 days; aggregates longer. You may request export or deletion of your account data.',
      'Patreon linking requests the smallest scopes needed to check your membership tier. We never see payment details.',
      'Draft wording pending legal review.',
    ],
  },
  content: {
    title: 'Content policy',
    body: [
      'Original works or properly authorized adaptations only. No plagiarism.',
      'Mass-produced AI-generated narrative is not published. AI assistance for outlines, proofreading, translation help or brainstorming is fine when a human remains responsible for the story and its rights.',
      'Content ratings and warnings help readers choose; unlawful content is excluded.',
      'Draft wording pending legal review.',
    ],
  },
  community: {
    title: 'Community guidelines',
    body: [
      'Be kind to authors and other readers. Criticism of a story is welcome; harassment is not.',
      'Mark spoilers. Do not post spam, link farms or repeated messages.',
      'Reports are private and reviewed by people. Moderation decisions are logged and can be appealed.',
    ],
  },
  copyright: {
    title: 'Copyright and takedowns',
    body: [
      'If you believe material here infringes your rights, send a report with the location of the material and evidence of your rights.',
      'Reports are reviewed by a person; we may ask for more information before acting.',
      'Contact details are published before the platform accepts invited authors.',
    ],
  },
};

export function PolicyPage(): m.Component {
  return {
    view: () => {
      const p = POLICIES[m.route.param('kind') ?? ''];
      if (!p) return m('.page-text', [pageHeader('Policy not found'), m(m.route.Link, { href: '/policies/terms' }, 'Terms of use')]);
      setTitle(p.title);
      return m('.page-text', [pageHeader(p.title), p.body.map((t) => m('p', t))]);
    },
  };
}
