class Outer:
    class Inner:
        def validate(self):
            return True

    def validate(self):
        return self.Inner().validate()

    def process(self):
        return None


class AnotherOuter:
    def validate(self):
        return False
