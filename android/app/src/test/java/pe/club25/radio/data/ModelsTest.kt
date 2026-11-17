package pe.club25.radio.data

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.net.URL

/** Lectura de /api/app/config y /api/status (mismo formato que docs/api.md). */
class ModelsTest {
    private val base = "http://10.0.2.2:8120"
    private fun resolve(u: String) = URL(URL("$base/"), u).toString()

    @Test
    fun parseConfig() {
        val json = """
            {"name":"Radio Club 25","slogan":"La música que va contigo","logo_url":"/static/img/brand/logo.svg",
             "icon_url":"C:/ruta/mala.svg",
             "colors":{"navy":"#0A1A6B","royal":"#223CB8","red":"#ff1e1e","orange":"naranja"},
             "radio_enabled":true,
             "stations":[{"id":1,"label":"93.3 FM","city":"Huacho","listen_url":"/radio","external":false},
                         {"id":3,"label":"105.4 FM","city":"Huaral","listen_url":"https://stream.ejemplo.pe/huaral","external":true}],
             "tv":{"enabled":true,"hls_url":"/hls/tv/index.m3u8"},
             "contact":{"phone":"01 234 5678","whatsapp":"+51 999 888 777","email":"","address":"","office_hours":""},
             "social":{"facebook":"https://facebook.com/club25","instagram":"","tiktok":"","youtube":"","x":""},
             "app":{"notice":"Aviso","notice_url":"","min_version":3,"play_store_url":"","privacy_url":""},
             "status_url":"/api/status"}
        """.trimIndent()
        val c = AppConfig.parse(json, ::resolve)
        assertEquals("Radio Club 25", c.name)
        assertEquals("$base/static/img/brand/logo.svg", c.logoUrl)
        assertEquals("", c.iconUrl) // URL inválida: se ignora sin romper la configuración
        assertEquals(0xFFFF1E1E, c.colors.red)
        assertEquals(BrandColors().orange, c.colors.orange) // color inválido → el de la marca
        assertEquals(2, c.stations.size)
        assertEquals("$base/radio", c.stations[0].listenUrl)
        assertTrue(c.stations[1].external)
        assertEquals("https://stream.ejemplo.pe/huaral", c.stations[1].listenUrl)
        assertTrue(c.tvEnabled)
        assertEquals("$base/hls/tv/index.m3u8", c.tvHlsUrl)
        assertEquals(listOf("WhatsApp", "Llamar", "Facebook"), c.links.map { it.label })
        assertEquals("https://wa.me/51999888777", c.links[0].url)
        assertEquals(3, c.minVersion)
        assertEquals("$base/api/status", c.statusUrl)
    }

    @Test
    fun parseStatus() {
        val json = """
            {"radio":{"online":true},"tv":{"online":false,"viewers":0},
             "stations":[{"id":1,"online":true,"listeners":12,"title":"Artista - Canción",
                          "on_air":{"title":"Mañanas Club","host":"DJ"}},
                         {"id":3,"external":true,"online":null,"listeners":null,"title":"","on_air":null}]}
        """.trimIndent()
        val s = LiveStatus.parse(json)
        assertFalse(s.tvOnline)
        assertEquals(true, s.stations[1]?.online)
        assertEquals(12, s.stations[1]?.listeners)
        assertEquals("Mañanas Club · DJ", s.stations[1]?.program)
        assertNull(s.stations[3]?.online)
        assertNull(s.stations[3]?.listeners)
    }
}
