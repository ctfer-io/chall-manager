---
title: The Need
description: Understand why it was a problem before chall-manager.
categories: [References]
weight: 2
---

"Sharing is caring" they said... they were wrong.
Sometimes, isolating things can get spicy, but it implies replicating infrastructures. If someone soft-locks its challenge, it would be its entire problem: it won't affect other players experience.
Furthermore, you could then imagine a network challenge where your goal is to chain a vulnerability to a Man-in-the-Middle attack to get access to a router and spy on communications to steal a flag ! Or a small company infrastructure that you would have to break into and get an administrator account !
And what if the infrastructure went down ? Then it would be isolated, other players could still play in their own environment.
This idea can be really productive, enhance the possibilities of a CTF and enable realistic -if not realism- attacks to take place in a controlled manner.

This is the **Challenge on Demand** problem: giving every player or team its own instance of a challenge, isolated, to play complex scenarios.
With a solution to this problem, players and teams could use a brand new set of knowledges they could not until this: pivoting from an AWS infrastructure to an on-premise company IT system, break into a virtual bank vault to squeeze out all the belongings, hack their own Industrial Control System or IoT assets...
A solution to this problem would open a myriad possibilites.

## The existing solutions

As it is a widespread known limitation in the community, people tried to solve the problem.
They conceived solutions that would fit their need, considering a set of functionalities and requirements, then built, released and deployed them successfully.

Some of them are famous:
- [CTFd whale](https://github.com/frankli0324/ctfd-whale) is a [CTFd](https://ctfd.io) plugin able to spin up Docker containers on demand.
- [CTFd owl](https://github.com/D0g3-Lab/H1ve/tree/master/CTFd/plugins/ctfd-owl) is an alternative to CTFd whale, less famous.
- [KubeCTF](https://github.com/DownUnderCTF/ctfd-kubectf-plugin) is another [CTFd](https://ctfd.io) plugin made to spin up Kubernetes environments.
- [Klodd](https://klodd.tjcsec.club/) is a [rCTF](https://rctf.redpwn.net/) service also made to spin up Kubernetes environments.

Nevertheless, they partially solved the root problem: those solutions solved the problem in a context (Docker, Kubernetes), with a Domain Specific Language (DSL) that does not guarantee non-vendor-lock-in nor ease use and testing, and lack functionalities such as [Hot Update](/docs/chall-manager/design/hot-update).

An ideal solution to this problem require:
- the use of a programmatic language, not a DSL (non-vendor-lock-in and functionalities)
- the capacity to deploy an instance without the solution itself (non-vendor-lock-in)
- the capacity to use largely-adopted technologies (e.g. Terraform, Ansible, etc., for functionalities)
- the genericity of its approach to avoid re-implementing the solution for every service provider (functionalities)

There were no existing solutions that fits those requirements... Until now.

## Grey litterature survey

Follows an exhaustive grey litterature survey on the solutions made to solve the Challenge on Demand problem.
To enhance this one, please open an issue or a pull request, we would be glad to improve it !

<table>
    <tr align="center"><th>Service</th><th>CTF platform</th><th>Genericity</th><th>Technical approach</th><th>Scalable</th></tr>
    <tr>
        <!--Service-->
        <td><a href="https://github.com/frankli0324/ctfd-whale">CTFd whale</a></td>
        <!--CTF platform-->
        <td><a href="https://ctfd.io">CTFd</a></td>
        <!--Genericity-->
        <td align="center">❌</td>
        <!--Technical approach-->
        <td>Docker socket</td>
        <!--Scalable-->
        <td align="center">❌¹</td>
    </tr><tr>
        <!--Service-->
        <td><a href="https://github.com/D0g3-Lab/H1ve/tree/master/CTFd/plugins/ctfd-owl">CTFd owl</a></td>
        <!--CTF platform-->
        <td><a href="https://ctfd.io">CTFd</a></td>
        <!--Genericity-->
        <td align="center">❌</td>
        <!--Technical approach-->
        <td>Docker socket</td>
        <!--Scalable-->
        <td align="center">❌¹</td>
    </tr><tr>
        <!--Service-->
        <td><a href="https://github.com/DownUnderCTF/kube-ctf">KubeCTF</a></td>
        <!--CTF platform-->
        <td>Agnostic, <a href="https://github.com/DownUnderCTF/ctfd-kubectf-plugin">CTFd plugin</a></td>
        <!--Genericity-->
        <td align="center">❌</td>
        <!--Technical approach-->
        <td>Use Kubernetes API and annotations</td>
        <!--Scalable-->
        <td align="center">✅</td>
    </tr><tr>
        <!--Service-->
        <td><a href="https://klodd.tjcsec.club/">Klodd</a></td>
        <!--CTF platform-->
        <td><a href="https://rctf.redpwn.net/">rCTF</a></td>
        <!--Genericity-->
        <td align="center">❌</td>
        <!--Technical approach-->
        <td>Use Kubernetes CRDs and a microservice</td>
        <!--Scalable-->
        <td align="center">✅</td>
    </tr><tr>
        <!--Service-->
        <td><a href="https://github.com/4T-24/i">4T$'s I</a></td>
        <!--CTF platform-->
        <td><a href="https://ctfd.io">CTFd</a></td>
        <!--Genericity-->
        <td align="center">❌</td>
        <!--Technical approach-->
        <td>Use Kubernetes CRDs and a microservice</td>
        <!--Scalable-->
        <td align="center">✅</td>
    </tr>
</table>

¹ not considered scalable as it reuses a Docker socket thus require a whole VM. As such a VM is not scaled automatically (despite being technically feasible), the property aligns with the limitation.

Classification for `Scalable`:
- ✅ partially or completfully scalable. Classification does not goes further on criteria as the time to scale, autoscaling, load balancing, anticipated scaling, descaling, etc.
- ❌ only one instance is possible.

## Litterature

More than a technical problem, the chall-manager also provides a solution to a scientific problem. In previous approaches for cybersecurity competitions, many referred to an hypothetic generic approach to the [Challenge on Demand](/docs/chall-manager/glossary#challenge-on-demand) problem.
None of them introduced a solution or even a realistic approach, until ours.

In those approaches to Challenge on Demand, we find:
- ["PAIDEUSIS: A Remote Hybrid Cyber Range for Hardware, Network, and IoT Security Training", Bera et al. (2021)](https://ceur-ws.org/Vol-2940/paper24.pdf) provide Challenge on Demand for Hardware systems (Industrial Control Systems, FPGA).
- ["Design of Remote Service Infrastructures for Hardware-based Capture-the-Flag Challenges", Marongiu and Perra (2021)](https://webthesis.biblio.polito.it/secure/21134/1/tesi.pdf) related to PAIDEUSIS, with the foundations for hardware-based Challenge on Demand.
- ["Lowering the Barriers to Capture the Flag administration and Participation", Kevin Chung (2017)](https://www.usenix.org/system/files/conference/ase17/ase17_paper_chung.pdf) in which it is a limitation of CTFd, where picoCTF has an "autogen" challenge feature.
- ["Automatic Problem Generation for Capture-the-Flag Competitions", Burket et al. (2015)](https://www.usenix.org/conference/3gse15/summit-program/presentation/burket) discusses the problem of generating challenges on demand, regarding fairness evaluation for players.
- ["Scalable Learning Environments for Teaching Cybersecurity Hands-on", Vykopal et al. (2021)](https://doi.org/10.1109/FIE49875.2021.9637180) discusses the problem of creating cybersecurity environments, similarly to Challenge on Demand, and provide guidelines for production environments at scale.
- ["GENICS: A framework for Generating Attack Scenarios for Cybersecurity Exercices on Industrial Control Systems", Song et al. (2024)](https://doi.org/10.3390/app14020768) formulates a process to generate attack scenario applicable in a CTF context, technically possible through approaches like PAIDEUSIS.

## Conclusions

Even if there are some solutions developed to help the community deal with the Challenge on Demand problem, we can see many limitations: only Docker and Kubernetes are covered, and none is able to provide the required genericity to consider other providers, even custom ones.

A production-ready solution would enable the community to explore new kinds of challenges, both technical and scientific.

This is why we created chall-manager: provide a free, open-source, generic, non-vendor-lock-in and ready-for-production solution.
Feel free to build on top of it. Change the game.

## What's next ?

How we tackled down the complexity of building this system, starting from [the architecture](/docs/chall-manager/design/architecture).
