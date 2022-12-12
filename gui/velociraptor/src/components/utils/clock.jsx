import "./clock.css";

import React, { Component } from 'react';
import VeloTimestamp from "./time.jsx";

const POLL_TIME = 1000; // 1 sec

export default class VeloLiveClock extends Component {
    componentDidMount = () => {
        this.interval = setInterval(()=>{
            this.setState({date: new Date()});
        }, POLL_TIME);
    }

    componentWillUnmount() {
        clearInterval(this.interval);
    }

    state = {
        date: new Date(),
    }

    render() {
        return (
            <VeloTimestamp
              iso={this.state.date}
              className="float-right"/>
        );
    }
}
