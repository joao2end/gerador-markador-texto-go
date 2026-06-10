include <BOSL2/std.scad>

texto = "JÚLIA";
espessura_relevo = 2.5;

diametro_furo = 8.2;
espessura_parede = 2;

raio_furo = diametro_furo / 2;
raio_tubo = raio_furo + espessura_parede;
raio_externo = raio_tubo + espessura_relevo;

comprimento_tubo = 100;
largura_texto = 0.7;
$fn = 40;

palavras = str_split(texto, " ");
lado1 = palavras[0];
lado2 = len(palavras) >= 2 ? palavras[1] : "";
max_chars = max(len(lado1), len(lado2));
tamanho_fonte = min(largura_texto * comprimento_tubo / (max(1, max_chars) * 0.65), PI * raio_tubo);

module escrever_lado(txt, angulo) {
    rotate(angulo)
    cylindrical_extrude(ir = raio_tubo - 0.1, or = raio_externo) {
        rotate(-90)
        text(txt, size = tamanho_fonte, font = "Arial:style=Bold", halign = "center", valign = "center");
    }
}

rotate([0, 90, 0])
difference() {
    union() {
        cylinder(h = comprimento_tubo, r = raio_tubo, center = true);
        escrever_lado(lado1, 0);
        escrever_lado(lado2, 180);
    }

    cylinder(h = comprimento_tubo + 2, r = raio_furo, center = true);
}
