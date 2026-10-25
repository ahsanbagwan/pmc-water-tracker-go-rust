package org.punewatertracker.repository;

import org.punewatertracker.model.Locality;
import org.punewatertracker.model.WaterStatus;
import org.springframework.data.jpa.repository.JpaRepository;

import java.util.List;

public interface LocalityRepository extends JpaRepository<Locality, Long> {

    List<Locality> findByStatus(WaterStatus status);

    List<Locality> findByVerifiedTrue();

    /** Most recently verified/updated entries first -- feeds the RSS endpoint. */
    List<Locality> findByVerifiedTrueOrderByLastVerifiedDesc();

    List<Locality> findByNameContainingIgnoreCaseAndVerifiedTrue(String name);

    boolean existsByName(String name);
}
