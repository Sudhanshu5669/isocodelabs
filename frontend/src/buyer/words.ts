import type { Lang } from '../api/types'
import type { PassWords } from '../components/VisitPass'

// Every word on the buyer's chat and visit pages, in English, Hindi and
// Gujarati. The assistant replies in the buyer's language, so the buttons
// around it should too.

export type Words = {
  chatTitle: string
  chatIntro: string
  project: string
  language: string
  name: string
  phone: string
  phoneHint: string
  consent: string
  start: string
  starting: string
  pickedSlot: (when: string, who: string) => string
  messagePlaceholder: string
  send: string
  talkToPerson: string
  thinking: string
  online: string
  typing: string
  handedOff: (name: string) => string
  handedOffTeam: string
  freeTimes: string
  bookedTitle: string
  openVisit: string
  quick: string[]
  retry: string
  // visit page
  visitTitle: string
  visitHello: (name: string) => string
  reschedule: string
  pickNewTime: string
  cancelVisit: string
  cancelConfirm: string
  keepVisit: string
  cancelled: string
  rescheduled: string
  visitClosed: string
  stopMessages: string
  stopped: string
  stopExplain: string
  noFreeTimes: string
  close: string
  pass: PassWords
  withLabel: string
}

const tz = 'Asia/Kolkata'
const longDay = (locale: string) => (iso: string) =>
  new Intl.DateTimeFormat(locale, { timeZone: tz, weekday: 'long', day: 'numeric', month: 'long' }).format(new Date(iso))

export const words: Record<Lang, Words> = {
  en: {
    chatTitle: 'Ask about your new home',
    chatIntro: 'Our assistant answers from the project fact sheet, any time, and books a site visit in a free slot. A salesperson takes over whenever you like.',
    project: 'Project',
    language: 'Language',
    name: 'Your name',
    phone: 'Mobile number',
    phoneHint: 'A 10-digit Indian mobile number. We use it to confirm your visit.',
    consent: 'I agree that Company A may contact me about this enquiry, including visit reminders. I can reply STOP at any time.',
    start: 'Start the chat',
    starting: 'Starting',
    pickedSlot: (when, who) => `You picked ${when} with ${who}. We book it as soon as the chat starts.`,
    messagePlaceholder: 'Ask about prices, sizes, possession, or book a visit',
    send: 'Send',
    talkToPerson: 'Talk to a person',
    thinking: 'Writing a reply',
    online: 'online',
    typing: 'typing…',
    handedOff: (n) => `${n} from our sales team has your details and will call you.`,
    handedOffTeam: 'Our sales team has your details and will call you.',
    freeTimes: 'Free visit times',
    bookedTitle: 'Your visit is booked',
    openVisit: 'View, reschedule or cancel',
    quick: ['Show free visit times', 'When is possession?', 'Is there parking?', 'What sizes are there?'],
    retry: 'Try again',
    visitTitle: 'Your site visit',
    visitHello: (n) => `Hello ${n}, here is your visit.`,
    reschedule: 'Choose another time',
    pickNewTime: 'Pick a new time. Your current booking stays until you pick one.',
    cancelVisit: 'Cancel the visit',
    cancelConfirm: 'Cancel this visit? The time will be offered to someone else.',
    keepVisit: 'Keep it',
    cancelled: 'Your visit is cancelled. You can book again from the chat at any time.',
    rescheduled: 'Done. Your visit has moved to the new time.',
    visitClosed: 'This visit can no longer be changed.',
    stopMessages: 'Stop all messages',
    stopped: 'You will not get any more messages from us.',
    stopExplain: 'We send a reminder 1 day and 2 hours before your visit.',
    noFreeTimes: 'No other free times in the next two weeks. Ask in the chat and a salesperson will arrange one.',
    close: 'Close',
    withLabel: 'with',
    pass: {
      title: 'Site visit',
      with: 'You will meet',
      where: 'Where',
      openMap: 'Open location in Maps',
      rera: 'RERA reg. no.',
      cancelled: 'Cancelled',
      formatDay: longDay('en-IN'),
    },
  },
  hi: {
    chatTitle: 'अपने नए घर के बारे में पूछें',
    chatIntro: 'हमारा असिस्टेंट प्रोजेक्ट की फ़ैक्ट शीट से किसी भी समय जवाब देता है और खाली स्लॉट में साइट विज़िट बुक करता है। जब चाहें, सेल्स टीम का कोई व्यक्ति बात संभाल लेगा।',
    project: 'प्रोजेक्ट',
    language: 'भाषा',
    name: 'आपका नाम',
    phone: 'मोबाइल नंबर',
    phoneHint: '10 अंकों का भारतीय मोबाइल नंबर। इससे हम आपकी विज़िट की पुष्टि करते हैं।',
    consent: 'मैं सहमत हूँ कि Company A इस पूछताछ के बारे में, विज़िट रिमाइंडर सहित, मुझसे संपर्क कर सकती है। मैं कभी भी STOP लिखकर संदेश बंद कर सकता/सकती हूँ।',
    start: 'चैट शुरू करें',
    starting: 'शुरू हो रहा है',
    pickedSlot: (when, who) => `आपने ${who} के साथ ${when} चुना है। चैट शुरू होते ही हम इसे बुक कर देंगे।`,
    messagePlaceholder: 'कीमत, साइज़, पज़ेशन के बारे में पूछें या विज़िट बुक करें',
    send: 'भेजें',
    talkToPerson: 'किसी व्यक्ति से बात करें',
    thinking: 'जवाब लिखा जा रहा है',
    online: 'ऑनलाइन',
    typing: 'टाइप कर रहे हैं…',
    handedOff: (n) => `हमारी सेल्स टीम के ${n} के पास आपकी जानकारी है और वे आपको कॉल करेंगे।`,
    handedOffTeam: 'हमारी सेल्स टीम के पास आपकी जानकारी है और वे आपको कॉल करेंगे।',
    freeTimes: 'विज़िट के खाली समय',
    bookedTitle: 'आपकी विज़िट बुक हो गई है',
    openVisit: 'देखें, समय बदलें या रद्द करें',
    quick: ['विज़िट के खाली समय दिखाएँ', 'पज़ेशन कब है?', 'क्या पार्किंग है?', 'कौन-से साइज़ हैं?'],
    retry: 'फिर से कोशिश करें',
    visitTitle: 'आपकी साइट विज़िट',
    visitHello: (n) => `नमस्ते ${n}, यह रही आपकी विज़िट।`,
    reschedule: 'दूसरा समय चुनें',
    pickNewTime: 'नया समय चुनें। जब तक आप नया समय नहीं चुनते, मौजूदा बुकिंग बनी रहेगी।',
    cancelVisit: 'विज़िट रद्द करें',
    cancelConfirm: 'क्या यह विज़िट रद्द करें? यह समय किसी और को दिया जाएगा।',
    keepVisit: 'रहने दें',
    cancelled: 'आपकी विज़िट रद्द हो गई है। आप चैट से कभी भी फिर से बुक कर सकते हैं।',
    rescheduled: 'हो गया। आपकी विज़िट नए समय पर है।',
    visitClosed: 'यह विज़िट अब बदली नहीं जा सकती।',
    stopMessages: 'सभी संदेश बंद करें',
    stopped: 'अब आपको हमसे कोई संदेश नहीं मिलेगा।',
    stopExplain: 'हम आपकी विज़िट से 1 दिन और 2 घंटे पहले रिमाइंडर भेजते हैं।',
    noFreeTimes: 'अगले दो हफ़्तों में कोई और खाली समय नहीं है। चैट में पूछें, सेल्स टीम समय तय कर देगी।',
    close: 'बंद करें',
    withLabel: 'के साथ',
    pass: {
      title: 'साइट विज़िट',
      with: 'आपसे मिलेंगे',
      where: 'कहाँ',
      openMap: 'मैप में लोकेशन खोलें',
      rera: 'RERA पंजीकरण सं.',
      cancelled: 'रद्द',
      formatDay: longDay('hi-IN'),
    },
  },
  gu: {
    chatTitle: 'તમારા નવા ઘર વિશે પૂછો',
    chatIntro: 'અમારો આસિસ્ટન્ટ પ્રોજેક્ટની ફેક્ટ શીટ પરથી ગમે ત્યારે જવાબ આપે છે અને ખાલી સ્લોટમાં સાઇટ વિઝિટ બુક કરે છે. તમે ઇચ્છો ત્યારે સેલ્સ ટીમના વ્યક્તિ વાત સંભાળી લેશે.',
    project: 'પ્રોજેક્ટ',
    language: 'ભાષા',
    name: 'તમારું નામ',
    phone: 'મોબાઇલ નંબર',
    phoneHint: '10 અંકનો ભારતીય મોબાઇલ નંબર. તેનાથી અમે તમારી વિઝિટની પુષ્ટિ કરીએ છીએ.',
    consent: 'હું સંમત છું કે Company A આ પૂછપરછ વિશે, વિઝિટ રિમાઇન્ડર સહિત, મારો સંપર્ક કરી શકે છે. હું ગમે ત્યારે STOP લખીને સંદેશા બંધ કરી શકું છું.',
    start: 'ચેટ શરૂ કરો',
    starting: 'શરૂ થઈ રહ્યું છે',
    pickedSlot: (when, who) => `તમે ${who} સાથે ${when} પસંદ કર્યું છે. ચેટ શરૂ થતાં જ અમે તે બુક કરી દઈશું.`,
    messagePlaceholder: 'કિંમત, સાઇઝ, પઝેશન વિશે પૂછો અથવા વિઝિટ બુક કરો',
    send: 'મોકલો',
    talkToPerson: 'કોઈ વ્યક્તિ સાથે વાત કરો',
    thinking: 'જવાબ લખાઈ રહ્યો છે',
    online: 'ઑનલાઇન',
    typing: 'ટાઇપ કરી રહ્યા છે…',
    handedOff: (n) => `અમારી સેલ્સ ટીમના ${n} પાસે તમારી વિગતો છે અને તેઓ તમને કૉલ કરશે.`,
    handedOffTeam: 'અમારી સેલ્સ ટીમ પાસે તમારી વિગતો છે અને તેઓ તમને કૉલ કરશે.',
    freeTimes: 'વિઝિટના ખાલી સમય',
    bookedTitle: 'તમારી વિઝિટ બુક થઈ ગઈ છે',
    openVisit: 'જુઓ, સમય બદલો અથવા રદ કરો',
    quick: ['વિઝિટના ખાલી સમય બતાવો', 'પઝેશન ક્યારે છે?', 'પાર્કિંગ છે?', 'કયા સાઇઝ છે?'],
    retry: 'ફરી પ્રયાસ કરો',
    visitTitle: 'તમારી સાઇટ વિઝિટ',
    visitHello: (n) => `નમસ્તે ${n}, આ રહી તમારી વિઝિટ.`,
    reschedule: 'બીજો સમય પસંદ કરો',
    pickNewTime: 'નવો સમય પસંદ કરો. તમે નવો સમય પસંદ ન કરો ત્યાં સુધી હાલની બુકિંગ રહેશે.',
    cancelVisit: 'વિઝિટ રદ કરો',
    cancelConfirm: 'આ વિઝિટ રદ કરવી છે? આ સમય બીજા કોઈને આપવામાં આવશે.',
    keepVisit: 'રહેવા દો',
    cancelled: 'તમારી વિઝિટ રદ થઈ ગઈ છે. તમે ચેટમાંથી ગમે ત્યારે ફરી બુક કરી શકો છો.',
    rescheduled: 'થઈ ગયું. તમારી વિઝિટ નવા સમય પર છે.',
    visitClosed: 'આ વિઝિટ હવે બદલી શકાતી નથી.',
    stopMessages: 'બધા સંદેશા બંધ કરો',
    stopped: 'હવે તમને અમારા તરફથી કોઈ સંદેશ નહીં મળે.',
    stopExplain: 'અમે તમારી વિઝિટના 1 દિવસ અને 2 કલાક પહેલાં રિમાઇન્ડર મોકલીએ છીએ.',
    noFreeTimes: 'આગામી બે અઠવાડિયામાં બીજો કોઈ ખાલી સમય નથી. ચેટમાં પૂછો, સેલ્સ ટીમ સમય નક્કી કરશે.',
    close: 'બંધ કરો',
    withLabel: 'સાથે',
    pass: {
      title: 'સાઇટ વિઝિટ',
      with: 'તમને મળશે',
      where: 'ક્યાં',
      openMap: 'મેપમાં લોકેશન ખોલો',
      rera: 'RERA નોંધણી નં.',
      cancelled: 'રદ',
      formatDay: longDay('gu-IN'),
    },
  },
}

export const languageNames: Record<Lang, string> = { en: 'English', hi: 'हिंदी', gu: 'ગુજરાતી' }

/** Short day for slot headings, in the buyer's language. */
export const shortDay = (lang: Lang) => (iso: string) =>
  new Intl.DateTimeFormat(lang === 'en' ? 'en-IN' : `${lang}-IN`, { timeZone: tz, weekday: 'short', day: 'numeric', month: 'short' }).format(new Date(iso))
