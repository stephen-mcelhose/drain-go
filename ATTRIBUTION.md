# Attribution

This library implements the **Drain** algorithm described in:

> Pinjia He, Jieming Zhu, Zibin Zheng, and Michael R. Lyu.
> "Drain: An Online Log Parsing Approach with Fixed Depth Tree."
> *Proceedings of the IEEE International Conference on Web Services (ICWS)*, 2017.
> DOI: [10.1109/ICWS.2017.13](https://doi.org/10.1109/ICWS.2017.13)

The paper is available from the authors at:
<https://jiemingzhu.github.io/pub/pjhe_icws2017.pdf>

This implementation also consulted [Drain3](https://github.com/logpai/Drain3) (MIT License, Copyright IBM)
for production-oriented extensions including ordered masking, inference-only matching,
and bounded cluster storage via LRU eviction.
