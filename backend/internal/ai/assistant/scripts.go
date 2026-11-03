package assistant

import "strings"

// Fixed messages the assistant sends without asking the model. They are used
// when the reply must be instant (the greeting), when a reply was blocked by a
// guardrail, and when the model is unavailable. Each exists in the three
// languages the assistant speaks.
//
// The Hindi and Gujarati wording is a careful first draft; a native-speaking
// member of the sales team should review it before launch.

// Languages, matching models.LangEnglish / LangHindi / LangGujarati.
const (
	LangEnglish  = "en"
	LangHindi    = "hi"
	LangGujarati = "gu"
)

type script struct{ en, hi, gu string }

func (s script) in(lang string) string {
	switch lang {
	case LangHindi:
		return s.hi
	case LangGujarati:
		return s.gu
	}
	return s.en
}

var (
	greeting = script{
		en: "Hello {name}, welcome to {project}. I can answer questions about the project and book a site visit for you. Which apartment are you looking for: 2 BHK or 3 BHK?",
		hi: "नमस्ते {name}, {project} में आपका स्वागत है। मैं प्रोजेक्ट के बारे में आपके सवालों के जवाब दे सकती हूँ और आपके लिए साइट विज़िट बुक कर सकती हूँ। आप कौन सा फ़्लैट देख रहे हैं: 2 BHK या 3 BHK?",
		gu: "નમસ્તે {name}, {project} માં આપનું સ્વાગત છે. હું પ્રોજેક્ટ વિશેના આપના પ્રશ્નોના જવાબ આપી શકું છું અને આપના માટે સાઇટ વિઝિટ બુક કરી શકું છું. આપ કયો ફ્લેટ શોધી રહ્યા છો: 2 BHK કે 3 BHK?",
	}

	// Sent instead of a reply that a guardrail blocked.
	priceSafe = script{
		en: "The final price and any offers are confirmed only by our sales team, so I can't quote them here. I can book a site visit, where the executive will go through pricing with you, or ask someone to call you. Which would you prefer?",
		hi: "अंतिम कीमत और कोई भी ऑफ़र केवल हमारी सेल्स टीम तय करती है, इसलिए मैं यहाँ उसकी जानकारी नहीं दे सकती। मैं आपकी साइट विज़िट बुक कर सकती हूँ, जहाँ एग्ज़ीक्यूटिव आपको कीमत समझाएँगे, या किसी से आपको कॉल करवा सकती हूँ। आप क्या पसंद करेंगे?",
		gu: "અંતિમ કિંમત અને કોઈપણ ઑફર ફક્ત અમારી સેલ્સ ટીમ જ નક્કી કરે છે, તેથી હું અહીં તે જણાવી શકતી નથી. હું આપની સાઇટ વિઝિટ બુક કરી શકું છું, જ્યાં એક્ઝિક્યુટિવ આપને કિંમત સમજાવશે, અથવા કોઈને આપને કૉલ કરવા કહી શકું છું. આપ શું પસંદ કરશો?",
	}

	// The model failed or timed out. The lead is already saved and a person has
	// been asked to follow up, so the buyer is never left without an answer.
	fallbackNamed = script{
		en: "Sorry, I can't answer properly right now. I've passed your details to {exec} from our sales team, who will call you soon. You can also pick a site-visit time below.",
		hi: "माफ़ कीजिए, मैं अभी ठीक से जवाब नहीं दे पा रही हूँ। मैंने आपकी जानकारी हमारी सेल्स टीम के {exec} को दे दी है, वे जल्द ही आपको कॉल करेंगे। आप नीचे साइट विज़िट का समय भी चुन सकते हैं।",
		gu: "માફ કરશો, હું અત્યારે બરાબર જવાબ આપી શકતી નથી. મેં આપની વિગતો અમારી સેલ્સ ટીમના {exec} ને આપી દીધી છે, તેઓ ટૂંક સમયમાં આપને કૉલ કરશે. આપ નીચે સાઇટ વિઝિટનો સમય પણ પસંદ કરી શકો છો.",
	}
	fallbackTeam = script{
		en: "Sorry, I can't answer properly right now. I've passed your details to our sales team, who will call you soon. You can also pick a site-visit time below.",
		hi: "माफ़ कीजिए, मैं अभी ठीक से जवाब नहीं दे पा रही हूँ। मैंने आपकी जानकारी हमारी सेल्स टीम को दे दी है, वे जल्द ही आपको कॉल करेंगे। आप नीचे साइट विज़िट का समय भी चुन सकते हैं।",
		gu: "માફ કરશો, હું અત્યારે બરાબર જવાબ આપી શકતી નથી. મેં આપની વિગતો અમારી સેલ્સ ટીમને આપી દીધી છે, તેઓ ટૂંક સમયમાં આપને કૉલ કરશે. આપ નીચે સાઇટ વિઝિટનો સમય પણ પસંદ કરી શકો છો.",
	}

	// The buyer pressed "talk to a person".
	handoffNamed = script{
		en: "Of course. {exec} from our sales team will call you shortly. Is there anything else I can help with meanwhile?",
		hi: "ज़रूर। हमारी सेल्स टीम के {exec} जल्द ही आपको कॉल करेंगे। तब तक क्या मैं आपकी और कोई मदद कर सकती हूँ?",
		gu: "ચોક્કસ. અમારી સેલ્સ ટીમના {exec} ટૂંક સમયમાં આપને કૉલ કરશે. ત્યાં સુધી શું હું આપની બીજી કોઈ મદદ કરી શકું?",
	}
	handoffTeam = script{
		en: "Of course. Someone from our sales team will call you shortly. Is there anything else I can help with meanwhile?",
		hi: "ज़रूर। हमारी सेल्स टीम से कोई जल्द ही आपको कॉल करेगा। तब तक क्या मैं आपकी और कोई मदद कर सकती हूँ?",
		gu: "ચોક્કસ. અમારી સેલ્સ ટીમમાંથી કોઈ ટૂંક સમયમાં આપને કૉલ કરશે. ત્યાં સુધી શું હું આપની બીજી કોઈ મદદ કરી શકું?",
	}

	// The buyer picked a slot card and it was booked.
	bookedMsg = script{
		en: "Your site visit is booked for {when} with {exec}. You'll get reminders a day before and two hours before. You can view, reschedule or cancel it from the visit link.",
		hi: "आपकी साइट विज़िट {when} को {exec} के साथ बुक हो गई है। आपको एक दिन पहले और दो घंटे पहले रिमाइंडर मिलेगा। विज़िट लिंक से आप इसे देख, बदल या रद्द कर सकते हैं।",
		gu: "આપની સાઇટ વિઝિટ {when} એ {exec} સાથે બુક થઈ ગઈ છે. આપને એક દિવસ પહેલાં અને બે કલાક પહેલાં રિમાઇન્ડર મળશે. વિઝિટ લિંક પરથી આપ તેને જોઈ, બદલી કે રદ કરી શકો છો.",
	}
)

// A booking that a person must confirm by phone.
var confirmByPhone = script{
	en: "Thank you. {exec} from our sales team will call you shortly to confirm your site-visit time.",
	hi: "धन्यवाद। हमारी सेल्स टीम के {exec} जल्द ही आपको कॉल करके साइट विज़िट का समय पक्का करेंगे।",
	gu: "આભાર. અમારી સેલ્સ ટીમના {exec} ટૂંક સમયમાં આપને કૉલ કરીને સાઇટ વિઝિટનો સમય નક્કી કરશે.",
}

// fill replaces {key} placeholders.
func fill(text string, kv ...string) string {
	return strings.NewReplacer(kv...).Replace(text)
}

// Greeting is the first message of every chat. It is sent instantly, without
// the model, so every enquiry gets an answer in well under the 2-minute target.
func Greeting(lang, firstName, project string) string {
	return fill(greeting.in(lang), "{name}", firstName, "{project}", project)
}

// PriceSafeReply replaces a reply that a guardrail blocked.
func PriceSafeReply(lang string) string { return priceSafe.in(lang) }

// FallbackReply is sent when the model is unavailable.
func FallbackReply(lang, execName string) string {
	if execName == "" {
		return fallbackTeam.in(lang)
	}
	return fill(fallbackNamed.in(lang), "{exec}", execName)
}

// HandoffReply confirms that a person will call.
func HandoffReply(lang, execName string) string {
	if execName == "" {
		return handoffTeam.in(lang)
	}
	return fill(handoffNamed.in(lang), "{exec}", execName)
}

// ConfirmByPhoneReply says a person will call to confirm the visit time.
func ConfirmByPhoneReply(lang, execName string) string {
	if execName == "" {
		execName = map[string]string{LangHindi: "हमारी टीम", LangGujarati: "અમારી ટીમ"}[lang]
		if execName == "" {
			execName = "Someone"
		}
	}
	return fill(confirmByPhone.in(lang), "{exec}", execName)
}

// BookedReply confirms a visit booked from a slot card.
func BookedReply(lang, when, execName string) string {
	return fill(bookedMsg.in(lang), "{when}", when, "{exec}", execName)
}
