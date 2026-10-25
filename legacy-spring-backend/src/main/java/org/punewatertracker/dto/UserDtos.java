package org.punewatertracker.dto;

import jakarta.validation.constraints.NotBlank;
import jakarta.validation.constraints.NotNull;
import jakarta.validation.constraints.Pattern;
import org.punewatertracker.model.Role;

public class UserDtos {

    public record CreateUserRequest(
            @NotBlank String username,

            @NotBlank
            @Pattern(
                    regexp = "^(?=.*[A-Za-z])(?=.*\\d).{8,}$",
                    message = "Password must be at least 8 characters and include at least one letter and one number"
            )
            String password,

            @NotNull Role role
    ) {}

    public record UserView(Long id, String username, Role role) {}
}
