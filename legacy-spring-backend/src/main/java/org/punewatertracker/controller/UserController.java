package org.punewatertracker.controller;

import jakarta.validation.Valid;
import org.punewatertracker.audit.Audited;
import org.punewatertracker.dto.UserDtos.CreateUserRequest;
import org.punewatertracker.dto.UserDtos.UserView;
import org.punewatertracker.model.User;
import org.punewatertracker.repository.UserRepository;
import org.springframework.http.HttpStatus;
import org.springframework.http.ResponseEntity;
import org.springframework.security.crypto.password.PasswordEncoder;
import org.springframework.web.bind.annotation.*;
import org.springframework.web.server.ResponseStatusException;

import java.util.List;

/** Admin-only: endpoint access is already restricted to ROLE_ADMIN in SecurityConfig. */
@RestController
@RequestMapping("/api/users")
public class UserController {

    private final UserRepository userRepository;
    private final PasswordEncoder passwordEncoder;

    public UserController(UserRepository userRepository, PasswordEncoder passwordEncoder) {
        this.userRepository = userRepository;
        this.passwordEncoder = passwordEncoder;
    }

    @GetMapping
    public List<UserView> list() {
        return userRepository.findAll().stream()
                .map(u -> new UserView(u.getId(), u.getUsername(), u.getRole()))
                .toList();
    }

    @PostMapping
    @Audited("CREATE_USER")
    public ResponseEntity<UserView> create(@Valid @RequestBody CreateUserRequest request) {
        if (userRepository.existsByUsername(request.username())) {
            throw new ResponseStatusException(HttpStatus.CONFLICT, "Username already taken");
        }
        User user = new User(request.username(), passwordEncoder.encode(request.password()), request.role());
        User saved = userRepository.save(user);
        return ResponseEntity.status(HttpStatus.CREATED)
                .body(new UserView(saved.getId(), saved.getUsername(), saved.getRole()));
    }

    @DeleteMapping("/{id}")
    @Audited("DELETE_USER")
    public ResponseEntity<Void> delete(@PathVariable Long id) {
        if (!userRepository.existsById(id)) {
            throw new ResponseStatusException(HttpStatus.NOT_FOUND, "No user with id " + id);
        }
        userRepository.deleteById(id);
        return ResponseEntity.noContent().build();
    }
}
